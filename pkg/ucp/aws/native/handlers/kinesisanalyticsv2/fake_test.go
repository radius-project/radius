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

package kinesisanalyticsv2

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// application is an application the fake knows.
type application struct {
	status  types.ApplicationStatus
	version int64

	// updating is how many more reads report the application as UPDATING after a change.
	updating int
}

// fakeAPI is an in-memory Kinesis Analytics v2 API for the CloudWatch logging options of the applications in apps. It
// keeps the logging options in an awsfake store, in the type's property format, keyed by their identifier, so the
// suite's Cloud Control sees the same resources. A change sets the application's version up by one and makes it
// report UPDATING for one read. Like AWS, a change sent with an old application version fails.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu   sync.Mutex
	apps map[string]*application
	next int

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{
		store: store,
		apps: map[string]*application{
			"app-one": {status: types.ApplicationStatusReady, version: 1},
			"app-two": {status: types.ApplicationStatusRunning, version: 4},
		},
		errs: map[string]error{},
	}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

var (
	// namePattern is the pattern of application names, which are 1 to 128 characters.
	namePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

	// optionIDPattern is the pattern of logging option IDs, which are 1 to 50 characters.
	optionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,50}$`)

	// tokenPattern is the pattern of conditional tokens, which are 1 to 512 characters.
	tokenPattern = regexp.MustCompile(`^[a-zA-Z0-9-_+/=]{1,512}$`)
)

// validARN reports whether arn is an ARN of 1 to 2048 characters, not bytes, as a log stream ARN must be.
func validARN(arn string) bool {
	return strings.HasPrefix(arn, "arn:") && utf8.RuneCountInString(arn) <= 2048
}

// validChange returns InvalidArgumentException, as AWS does, when a change names an application, logging option, or
// log stream outside the limits AWS's API reference gives, or sends a version ID or conditional token outside them.
func validChange(name *string, version *int64, token *string, ids []*string, arns []*string) error {
	valid := namePattern.MatchString(aws.ToString(name)) &&
		(version == nil || (*version >= 1 && *version <= 999999999)) &&
		(token == nil || tokenPattern.MatchString(*token))
	for _, id := range ids {
		valid = valid && optionIDPattern.MatchString(aws.ToString(id))
	}
	for _, arn := range arns {
		valid = valid && (arn == nil || validARN(*arn))
	}
	if !valid {
		return fault("InvalidArgumentException")
	}
	return nil
}

// optionID returns the logging option ID of the stored resource with the identifier.
func optionID(identifier string) string {
	if _, id, ok := strings.Cut(identifier, "|"); ok {
		return id
	}
	return identifier
}

// options returns the application's logging options, by their identifiers in the store.
func (f *fakeAPI) options(name string) map[string]types.CloudWatchLoggingOptionDescription {
	out := map[string]types.CloudWatchLoggingOptionDescription{}
	for _, identifier := range f.store.List(loggingOptionType) {
		props := f.store.Resource(loggingOptionType, identifier)
		if awsapi.String(props, "ApplicationName") != name {
			continue
		}
		out[identifier] = types.CloudWatchLoggingOptionDescription{
			CloudWatchLoggingOptionId: aws.String(optionID(identifier)),
			LogStreamARN:              aws.String(logStream(props)),
		}
	}
	return out
}

func (f *fakeAPI) descriptions(name string) []types.CloudWatchLoggingOptionDescription {
	options := f.options(name)
	out := []types.CloudWatchLoggingOptionDescription{}
	for _, identifier := range slices.Sorted(maps.Keys(options)) {
		out = append(out, options[identifier])
	}
	return out
}

// change makes a change to the application, after checking its version, failing it as the failer says.
func (f *fakeAPI) change(name *string, version *int64, apply func(app *application) error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	app, ok := f.apps[aws.ToString(name)]
	if !ok {
		return fault("ResourceNotFoundException")
	}
	if aws.ToInt64(version) != app.version {
		return fault("ConcurrentModificationException")
	}
	if err := apply(app); err != nil {
		return err
	}
	app.version++
	app.updating = 1
	return f.failer.After()
}

func (f *fakeAPI) DescribeApplication(ctx context.Context, in *kinesisanalyticsv2.DescribeApplicationInput, _ ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.DescribeApplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeApplication"]; err != nil {
		return nil, err
	}
	name := aws.ToString(in.ApplicationName)
	if !namePattern.MatchString(name) {
		return nil, fault("InvalidArgumentException")
	}
	app, ok := f.apps[name]
	if !ok {
		return nil, fault("ResourceNotFoundException")
	}
	status := app.status
	if app.updating > 0 {
		app.updating--
		status = types.ApplicationStatusUpdating
	}
	return &kinesisanalyticsv2.DescribeApplicationOutput{ApplicationDetail: &types.ApplicationDetail{
		ApplicationName:                     aws.String(name),
		ApplicationStatus:                   status,
		ApplicationVersionId:                aws.Int64(app.version),
		CloudWatchLoggingOptionDescriptions: f.descriptions(name),
	}}, nil
}

// ListApplications returns one application per page.
func (f *fakeAPI) ListApplications(ctx context.Context, in *kinesisanalyticsv2.ListApplicationsInput, _ ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.ListApplicationsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListApplications"]; err != nil {
		return nil, err
	}
	if (in.Limit != nil && (*in.Limit < 1 || *in.Limit > 50)) || (in.NextToken != nil && !namePattern.MatchString(*in.NextToken)) {
		return nil, fault("InvalidRequestException")
	}
	names := slices.Sorted(maps.Keys(f.apps))
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	out := &kinesisanalyticsv2.ListApplicationsOutput{}
	if start < len(names) {
		out.ApplicationSummaries = []types.ApplicationSummary{{ApplicationName: aws.String(names[start])}}
	}
	if start+1 < len(names) {
		out.NextToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}

func (f *fakeAPI) AddApplicationCloudWatchLoggingOption(ctx context.Context, in *kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionInput, _ ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["AddApplicationCloudWatchLoggingOption"]; err != nil {
		return nil, err
	}
	stream := aws.String("")
	if in.CloudWatchLoggingOption != nil {
		stream = in.CloudWatchLoggingOption.LogStreamARN
	}
	if err := validChange(in.ApplicationName, in.CurrentApplicationVersionId, in.ConditionalToken, nil, []*string{stream}); err != nil {
		return nil, err
	}
	name := aws.ToString(in.ApplicationName)
	err := f.change(in.ApplicationName, in.CurrentApplicationVersionId, func(app *application) error {
		f.next++
		identifier := fmt.Sprintf("%s|%d.1", name, f.next)
		f.store.Put(loggingOptionType, identifier, map[string]any{
			"Id":                      identifier,
			"ApplicationName":         name,
			"CloudWatchLoggingOption": map[string]any{"LogStreamARN": aws.ToString(stream)},
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionOutput{CloudWatchLoggingOptionDescriptions: f.descriptions(name)}, nil
}

// stored returns the store identifier of the application's logging option with the ID.
func (f *fakeAPI) stored(name string, id string) (string, error) {
	for identifier, o := range f.options(name) {
		if aws.ToString(o.CloudWatchLoggingOptionId) == id {
			return identifier, nil
		}
	}
	return "", fault("ResourceNotFoundException")
}

func (f *fakeAPI) DeleteApplicationCloudWatchLoggingOption(ctx context.Context, in *kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionInput, _ ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DeleteApplicationCloudWatchLoggingOption"]; err != nil {
		return nil, err
	}
	if err := validChange(in.ApplicationName, in.CurrentApplicationVersionId, in.ConditionalToken, []*string{in.CloudWatchLoggingOptionId}, nil); err != nil {
		return nil, err
	}
	err := f.change(in.ApplicationName, in.CurrentApplicationVersionId, func(app *application) error {
		identifier, err := f.stored(aws.ToString(in.ApplicationName), aws.ToString(in.CloudWatchLoggingOptionId))
		if err != nil {
			return err
		}
		f.store.Remove(loggingOptionType, identifier)
		return nil
	})
	return &kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionOutput{}, err
}

func (f *fakeAPI) UpdateApplication(ctx context.Context, in *kinesisanalyticsv2.UpdateApplicationInput, _ ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.UpdateApplicationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["UpdateApplication"]; err != nil {
		return nil, err
	}
	ids, arns := []*string{}, []*string{}
	for _, u := range in.CloudWatchLoggingOptionUpdates {
		ids, arns = append(ids, u.CloudWatchLoggingOptionId), append(arns, u.LogStreamARNUpdate)
	}
	if err := validChange(in.ApplicationName, in.CurrentApplicationVersionId, in.ConditionalToken, ids, arns); err != nil {
		return nil, err
	}
	err := f.change(in.ApplicationName, in.CurrentApplicationVersionId, func(app *application) error {
		for _, u := range in.CloudWatchLoggingOptionUpdates {
			identifier, err := f.stored(aws.ToString(in.ApplicationName), aws.ToString(u.CloudWatchLoggingOptionId))
			if err != nil {
				return err
			}
			props := f.store.Resource(loggingOptionType, identifier)
			props["CloudWatchLoggingOption"] = map[string]any{"LogStreamARN": aws.ToString(u.LogStreamARNUpdate)}
			f.store.Put(loggingOptionType, identifier, props)
		}
		return nil
	})
	return &kinesisanalyticsv2.UpdateApplicationOutput{}, err
}
