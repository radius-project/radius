/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package lambda

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
)

const (
	// account and region are the account and region of the fake's caller.
	account = "123456789012"
	region  = "us-west-2"

	// image is the name of the caller's MicroVM image, and imageArn its ARN. latestVersion is its latest version.
	image         = "web-app"
	imageArn      = "arn:aws:lambda:" + region + ":" + account + ":microvm-image:" + image
	latestVersion = "2"

	// pageSize is how many MicroVMs a ListMicrovms page has when the call sets no maxResults. AWS doesn't document
	// its default.
	pageSize = 2

	validationCode = "ValidationException"
	notFoundCode   = "ResourceNotFoundException"
)

// imageVersions are the versions of image.
var imageVersions = []string{"1", latestVersion}

var (
	rolePattern      = regexp.MustCompile(`^arn:aws[a-z\-]*:iam::[0-9]{12}:role/?[a-zA-Z_0-9+=,.@\-_/]+$`)
	logGroupPattern  = regexp.MustCompile(`^[a-zA-Z0-9_\-/.#]+$`)
	logStreamPattern = regexp.MustCompile(`^[^:*]*$`)
	noSpacePattern   = regexp.MustCompile(`^[^\s]+$`)
)

// fakeAPI is an in-memory AWS Lambda MicroVMs API. It keeps running MicroVMs in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources, and keeps the ones it terminates for its
// reads. A new MicroVM is PENDING for one read, then RUNNING. A terminated one is TERMINATING for one read, then
// TERMINATED. It follows the rules in microvmAPI.
type fakeAPI struct {
	store *awsfake.CloudControl

	mu sync.Mutex

	// next numbers the MicroVMs the fake runs.
	next int

	// state is the state of each MicroVM that isn't RUNNING, and reason why it is in it.
	state  map[string]types.MicrovmState
	reason map[string]string

	// terminated are the MicroVMs the fake terminated, as they were.
	terminated map[string]map[string]any

	// tokens maps each client token to the MicroVM its call ran.
	tokens map[string]string

	// runs counts the RunMicrovm calls that ran a MicroVM.
	runs int

	// failNext makes the next MicroVM the fake runs terminate before it is RUNNING, as one that fails to start.
	failNext bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store:      store,
		state:      map[string]types.MicrovmState{},
		reason:     map[string]string{},
		terminated: map[string]map[string]any{},
		tokens:     map[string]string{},
		errs:       map[string]error{},
	}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

// length reports whether v is min to max characters.
func length(v string, min int, max int) bool {
	n := utf8.RuneCountInString(v)
	return n >= min && n <= max
}

// validConnectors reports whether list has at most 10 connectors of 1 to 2048 characters.
func validConnectors(list []string) bool {
	return len(list) <= 10 && !slices.ContainsFunc(list, func(c string) bool { return !length(c, 1, 2048) })
}

// validRun returns ValidationException unless in meets the documented constraints.
func validRun(in *lambdamicrovms.RunMicrovmInput) error {
	valid := in.ImageIdentifier != nil && length(*in.ImageIdentifier, 1, 256) &&
		(in.ClientToken == nil || length(*in.ClientToken, 1, 128)) &&
		(in.ImageVersion == nil || length(*in.ImageVersion, 1, 2048) && noSpacePattern.MatchString(*in.ImageVersion)) &&
		(in.ExecutionRoleArn == nil || length(*in.ExecutionRoleArn, 20, 2048) && rolePattern.MatchString(*in.ExecutionRoleArn)) &&
		validConnectors(in.IngressNetworkConnectors) && validConnectors(in.EgressNetworkConnectors) &&
		(in.MaximumDurationInSeconds == nil || *in.MaximumDurationInSeconds >= 1 && *in.MaximumDurationInSeconds <= 28800) &&
		(in.RunHookPayload == nil || length(*in.RunHookPayload, 0, 4096))
	if p := in.IdlePolicy; p != nil {
		valid = valid && p.AutoResumeEnabled != nil && p.MaxIdleDurationSeconds != nil && *p.MaxIdleDurationSeconds >= 60 &&
			p.SuspendedDurationSeconds != nil && *p.SuspendedDurationSeconds >= 0
	}
	if l, ok := in.Logging.(*types.LoggingMemberCloudWatch); ok {
		group, stream := l.Value.LogGroup, l.Value.LogStream
		valid = valid && (group == nil || length(*group, 1, 512) && logGroupPattern.MatchString(*group)) &&
			(stream == nil || length(*stream, 1, 512) && logStreamPattern.MatchString(*stream))
	}
	if !valid {
		return fault(validationCode)
	}
	return nil
}

// resolveImage returns the ARN and version of the image the call names, by name or ARN, or ResourceNotFoundException.
func resolveImage(identifier string, version *string) (string, string, error) {
	if identifier != image && identifier != imageArn {
		return "", "", fault(notFoundCode)
	}
	if version == nil {
		return imageArn, latestVersion, nil
	}
	if !slices.Contains(imageVersions, *version) {
		return "", "", fault(notFoundCode)
	}
	return imageArn, *version, nil
}

func (f *fakeAPI) RunMicrovm(ctx context.Context, in *lambdamicrovms.RunMicrovmInput, _ ...func(*lambdamicrovms.Options)) (*lambdamicrovms.RunMicrovmOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["RunMicrovm"]; err != nil {
		return nil, err
	}
	if err := validRun(in); err != nil {
		return nil, err
	}
	if id, ok := f.tokens[aws.ToString(in.ClientToken)]; ok && in.ClientToken != nil {
		return f.runOutput(id), nil
	}
	_, version, err := resolveImage(aws.ToString(in.ImageIdentifier), in.ImageVersion)
	if err != nil {
		return nil, err
	}
	f.next++
	f.runs++
	id := fmt.Sprintf("mvm-%08d-0000-0000-0000-000000000000", f.next)
	stored := map[string]any{
		"MicrovmId":       id,
		"ImageIdentifier": aws.ToString(in.ImageIdentifier),
		"ImageVersion":    version,
		"Endpoint":        "https://" + id + ".microvm.lambda." + region + ".on.aws",
	}
	for name, v := range map[string]*string{"ExecutionRoleArn": in.ExecutionRoleArn, "RunHookPayload": in.RunHookPayload} {
		if v != nil {
			stored[name] = *v
		}
	}
	for name, list := range map[string][]string{"IngressNetworkConnectors": in.IngressNetworkConnectors, "EgressNetworkConnectors": in.EgressNetworkConnectors} {
		if len(list) > 0 {
			stored[name] = anyList(list)
		}
	}
	if p := in.IdlePolicy; p != nil {
		stored["IdlePolicy"] = map[string]any{
			"AutoResumeEnabled":        *p.AutoResumeEnabled,
			"MaxIdleDurationSeconds":   *p.MaxIdleDurationSeconds,
			"SuspendedDurationSeconds": *p.SuspendedDurationSeconds,
		}
	}
	if in.MaximumDurationInSeconds != nil {
		stored["MaximumDurationInSeconds"] = *in.MaximumDurationInSeconds
	}
	switch l := in.Logging.(type) {
	case *types.LoggingMemberDisabled:
		stored["Logging"] = map[string]any{"Disabled": map[string]any{}}
	case *types.LoggingMemberCloudWatch:
		cloudWatch := map[string]any{}
		for name, v := range map[string]*string{"LogGroup": l.Value.LogGroup, "LogStream": l.Value.LogStream} {
			if v != nil {
				cloudWatch[name] = *v
			}
		}
		stored["Logging"] = map[string]any{"CloudWatch": cloudWatch}
	}
	f.store.Put(microvmType, id, stored)
	f.state[id] = types.MicrovmStatePending
	if f.failNext {
		f.failNext = false
		f.reason[id] = "ApplicationStartupFailure"
	}
	if in.ClientToken != nil {
		f.tokens[*in.ClientToken] = id
	}
	return f.runOutput(id), nil
}

func anyList(list []string) []any {
	out := []any{}
	for _, v := range list {
		out = append(out, v)
	}
	return out
}

// describe returns the MicroVM as AWS describes it, or nil when AWS has no such MicroVM.
func (f *fakeAPI) describe(id string) *lambdamicrovms.GetMicrovmOutput {
	stored := f.store.Resource(microvmType, id)
	if stored == nil {
		stored = f.terminated[id]
	}
	if stored == nil {
		return nil
	}
	out := &lambdamicrovms.GetMicrovmOutput{MicrovmId: aws.String(id), State: types.MicrovmStateRunning}
	if state, ok := f.state[id]; ok {
		out.State = state
	}
	if reason := f.reason[id]; reason != "" {
		out.StateReason = aws.String(reason)
	}
	// AWS keeps the image the call named by its ARN.
	if v, ok := stored["ImageIdentifier"].(string); ok {
		out.ImageArn = aws.String(v)
		if v == image {
			out.ImageArn = aws.String(imageArn)
		}
	}
	for name, field := range map[string]**string{
		"ImageVersion": &out.ImageVersion, "ExecutionRoleArn": &out.ExecutionRoleArn, "Endpoint": &out.Endpoint,
	} {
		if v, ok := stored[name].(string); ok {
			*field = aws.String(v)
		}
	}
	out.IngressNetworkConnectors = stringList(stored["IngressNetworkConnectors"])
	out.EgressNetworkConnectors = stringList(stored["EgressNetworkConnectors"])
	if p, ok := stored["IdlePolicy"].(map[string]any); ok {
		out.IdlePolicy = &types.IdlePolicy{
			AutoResumeEnabled:        aws.Bool(p["AutoResumeEnabled"] == true),
			MaxIdleDurationSeconds:   aws.Int32(number(p["MaxIdleDurationSeconds"])),
			SuspendedDurationSeconds: aws.Int32(number(p["SuspendedDurationSeconds"])),
		}
	}
	if v, ok := stored["MaximumDurationInSeconds"]; ok {
		out.MaximumDurationInSeconds = aws.Int32(number(v))
	}
	return out
}

// stringList returns a stored list of strings.
func stringList(v any) []string {
	list, _ := v.([]any)
	out := []string{}
	for _, item := range list {
		out = append(out, fmt.Sprint(item))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// number returns a stored integer, which the suite's Cloud Control may store as a float64.
func number(v any) int32 {
	n, _ := strconv.Atoi(fmt.Sprint(v))
	return int32(n)
}

func (f *fakeAPI) runOutput(id string) *lambdamicrovms.RunMicrovmOutput {
	d := f.describe(id)
	return &lambdamicrovms.RunMicrovmOutput{
		MicrovmId: d.MicrovmId, ImageArn: d.ImageArn, ImageVersion: d.ImageVersion, ExecutionRoleArn: d.ExecutionRoleArn,
		IdlePolicy: d.IdlePolicy, IngressNetworkConnectors: d.IngressNetworkConnectors, EgressNetworkConnectors: d.EgressNetworkConnectors,
		MaximumDurationInSeconds: d.MaximumDurationInSeconds, Endpoint: d.Endpoint, State: d.State, StateReason: d.StateReason,
	}
}

// reported moves the state of a MicroVM a read returned on: a new one runs, or terminates when it fails to start, and
// a terminating one is terminated.
func (f *fakeAPI) reported(id string) {
	switch f.state[id] {
	case types.MicrovmStatePending:
		if f.reason[id] != "" {
			f.terminate(id)
			f.state[id] = types.MicrovmStateTerminated
			return
		}
		delete(f.state, id)
	case types.MicrovmStateTerminating:
		f.state[id] = types.MicrovmStateTerminated
	}
}

// terminate moves a MicroVM out of the store, as AWS's terminated MicroVMs aren't resources the type has.
func (f *fakeAPI) terminate(id string) {
	if stored := f.store.Resource(microvmType, id); stored != nil {
		f.terminated[id] = stored
		f.store.Remove(microvmType, id)
	}
}

func (f *fakeAPI) GetMicrovm(ctx context.Context, in *lambdamicrovms.GetMicrovmInput, _ ...func(*lambdamicrovms.Options)) (*lambdamicrovms.GetMicrovmOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["GetMicrovm"]; err != nil {
		return nil, err
	}
	id := aws.ToString(in.MicrovmIdentifier)
	if !length(id, 1, 256) {
		return nil, fault(validationCode)
	}
	out := f.describe(id)
	if out == nil {
		return nil, fault(notFoundCode)
	}
	f.reported(id)
	return out, nil
}

func (f *fakeAPI) TerminateMicrovm(ctx context.Context, in *lambdamicrovms.TerminateMicrovmInput, _ ...func(*lambdamicrovms.Options)) (*lambdamicrovms.TerminateMicrovmOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["TerminateMicrovm"]; err != nil {
		return nil, err
	}
	id := aws.ToString(in.MicrovmIdentifier)
	if !length(id, 1, 256) {
		return nil, fault(validationCode)
	}
	if f.describe(id) == nil {
		return nil, fault(notFoundCode)
	}
	if !gone(f.state[id]) {
		f.terminate(id)
		f.state[id] = types.MicrovmStateTerminating
	}
	return &lambdamicrovms.TerminateMicrovmOutput{}, nil
}

// ListMicrovms lists every MicroVM the fake has run, including terminated ones, maxResults or pageSize a page.
func (f *fakeAPI) ListMicrovms(ctx context.Context, in *lambdamicrovms.ListMicrovmsInput, _ ...func(*lambdamicrovms.Options)) (*lambdamicrovms.ListMicrovmsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListMicrovms"]; err != nil {
		return nil, err
	}
	size := pageSize
	if in.MaxResults != nil {
		if *in.MaxResults < 1 || *in.MaxResults > 50 {
			return nil, fault(validationCode)
		}
		size = int(*in.MaxResults)
	}
	ids := f.store.List(microvmType)
	for id := range f.terminated {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	end := min(start+size, len(ids))
	out := &lambdamicrovms.ListMicrovmsOutput{}
	for _, id := range ids[min(start, end):end] {
		d := f.describe(id)
		out.Items = append(out.Items, types.MicrovmItem{MicrovmId: d.MicrovmId, ImageArn: d.ImageArn, ImageVersion: d.ImageVersion, State: d.State})
	}
	if end < len(ids) {
		out.NextToken = aws.String(strconv.Itoa(end))
	}
	return out, nil
}
