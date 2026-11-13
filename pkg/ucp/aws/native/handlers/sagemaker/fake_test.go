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

package sagemaker

import (
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"
	"github.com/aws/smithy-go"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const arnPrefix = "arn:aws:sagemaker:us-west-2:123456789012:experiment-trial-component/"

// fakeAPI is an in-memory SageMaker API for trial components. It keeps them in an awsfake store, in the type's
// property format, so the suite's Cloud Control sees the same resources. A deleted trial component is still described
// once before it is gone.
type fakeAPI struct {
	store  *awsfake.CloudControl
	failer nativetest.Failer

	mu       sync.Mutex
	deleting map[string]bool

	// errs maps a method name to an error the call returns instead of acting.
	errs map[string]error
}

var _ API = (*fakeAPI)(nil)

func newFakeAPI(store *awsfake.CloudControl) *fakeAPI {
	return &fakeAPI{store: store, deleting: map[string]bool{}, errs: map[string]error{}}
}

func fault(code string) error {
	return &smithy.GenericAPIError{Code: code, Message: code, Fault: smithy.FaultClient}
}

var (
	// namePattern is the pattern of trial component names and display names, which are 1 to 120 characters.
	namePattern = regexp.MustCompile(`^[a-zA-Z0-9](-*[a-zA-Z0-9]){0,119}$`)

	// describeNamePattern is the pattern of the name DescribeTrialComponent takes: a trial component name, after an
	// optional ARN prefix.
	describeNamePattern = regexp.MustCompile(`^(arn:aws[a-z\-]*:sagemaker:[a-z0-9\-]*:[0-9]{12}:(experiment|experiment-trial|experiment-trial-component|artifact|action|context)/)?([a-zA-Z0-9](-*[a-zA-Z0-9]){0,119})$`)

	// resourceArnPattern is the pattern of the resource ARNs the tag calls take, which are at most 256 characters.
	resourceArnPattern = regexp.MustCompile(`^arn:aws[a-z-]*:sagemaker:[a-z0-9-]*:[0-9]{12}:.+$`)

	// tagPattern is the pattern of tag keys and values.
	tagPattern = regexp.MustCompile(`^[\p{L}\p{Z}\p{N}_.:/=+\-@]*$`)

	// mediaTypePattern is the pattern of an artifact's media type, which is at most 64 characters.
	mediaTypePattern = regexp.MustCompile(`^[-\w]+/[-\w+]+$`)
)

// validationError is the error SageMaker returns for input outside the limits its API reference gives.
var validationError = fault("ValidationError")

// chars returns the number of characters in s. SageMaker's length limits count characters, not bytes.
func chars(s string) int {
	return utf8.RuneCountInString(s)
}

// validName reports whether name is a valid trial component name or display name.
func validName(name string) bool {
	return chars(name) <= 120 && namePattern.MatchString(name)
}

// validTags returns ValidationError, as SageMaker does, unless there are at most 50 tags, each with a key of 1 to 128
// characters and a value, which is required, of at most 256 characters, of the tags' pattern.
func validTags(tags []types.Tag) error {
	if len(tags) > 50 {
		return validationError
	}
	for _, t := range tags {
		key, value := aws.ToString(t.Key), aws.ToString(t.Value)
		if t.Value == nil || chars(key) < 1 || chars(key) > 128 || chars(value) > 256 || !tagPattern.MatchString(key) || !tagPattern.MatchString(value) {
			return validationError
		}
	}
	return nil
}

// validResourceArn returns ValidationError, as SageMaker does, when arn isn't a SageMaker ARN of at most 256
// characters.
func validResourceArn(arn *string) error {
	if chars(aws.ToString(arn)) > 256 || !resourceArnPattern.MatchString(aws.ToString(arn)) {
		return validationError
	}
	return nil
}

// trialComponentValues are the values CreateTrialComponent and UpdateTrialComponent both take.
type trialComponentValues struct {
	name            *string
	displayName     *string
	status          *types.TrialComponentStatus
	parameters      map[string]types.TrialComponentParameterValue
	inputArtifacts  map[string]types.TrialComponentArtifact
	outputArtifacts map[string]types.TrialComponentArtifact
}

// valid returns ValidationError, as SageMaker does, when a value is outside the limits SageMaker's API reference
// gives for it.
func (v trialComponentValues) valid() error {
	if !validName(aws.ToString(v.name)) || (v.displayName != nil && !validName(*v.displayName)) {
		return validationError
	}
	if v.status != nil && (chars(aws.ToString(v.status.Message)) > 1024 ||
		(v.status.PrimaryStatus != "" && !slices.Contains(v.status.PrimaryStatus.Values(), v.status.PrimaryStatus))) {
		return validationError
	}
	if len(v.parameters) > 300 {
		return validationError
	}
	for key, value := range v.parameters {
		if s, ok := value.(*types.TrialComponentParameterValueMemberStringValue); chars(key) > 320 || (ok && chars(s.Value) > 2500) {
			return validationError
		}
	}
	for _, artifacts := range []map[string]types.TrialComponentArtifact{v.inputArtifacts, v.outputArtifacts} {
		if len(artifacts) > 60 {
			return validationError
		}
		for key, a := range artifacts {
			media := aws.ToString(a.MediaType)
			if chars(key) > 128 || a.Value == nil || chars(*a.Value) > 2048 || chars(media) > 64 || (a.MediaType != nil && !mediaTypePattern.MatchString(media)) {
				return validationError
			}
		}
	}
	return nil
}

// change makes a call that changes AWS, failing it as the failer says.
func (f *fakeAPI) change(apply func() error) error {
	if err := f.failer.Before(); err != nil {
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	return f.failer.After()
}

func (f *fakeAPI) CreateTrialComponent(ctx context.Context, in *sagemaker.CreateTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.CreateTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["CreateTrialComponent"]; err != nil {
		return nil, err
	}
	values := trialComponentValues{
		name:            in.TrialComponentName,
		displayName:     in.DisplayName,
		status:          in.Status,
		parameters:      in.Parameters,
		inputArtifacts:  in.InputArtifacts,
		outputArtifacts: in.OutputArtifacts,
	}
	if err := values.valid(); err != nil {
		return nil, err
	}
	if err := validTags(in.Tags); err != nil {
		return nil, err
	}
	if m := in.MetadataProperties; m != nil {
		for _, v := range []*string{m.CommitId, m.GeneratedBy, m.ProjectId, m.Repository} {
			if chars(aws.ToString(v)) > 1024 {
				return nil, validationError
			}
		}
	}
	name := aws.ToString(in.TrialComponentName)
	return &sagemaker.CreateTrialComponentOutput{TrialComponentArn: aws.String(arnPrefix + name)}, f.change(func() error {
		// Names are unique in the account without regard to case.
		for _, existing := range f.store.List(trialComponentType) {
			if strings.EqualFold(existing, name) {
				return fault("ResourceInUse")
			}
		}
		// The type has no hyperparameters or artifacts, so the fake doesn't keep them.
		in := *in
		in.Parameters, in.InputArtifacts, in.OutputArtifacts = nil, nil, nil
		props, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		f.store.Put(trialComponentType, name, props)
		return nil
	})
}

func (f *fakeAPI) DescribeTrialComponent(ctx context.Context, in *sagemaker.DescribeTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.DescribeTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["DescribeTrialComponent"]; err != nil {
		return nil, err
	}
	name := aws.ToString(in.TrialComponentName)
	if chars(name) > 256 || !describeNamePattern.MatchString(name) {
		return nil, validationError
	}
	name = strings.TrimPrefix(name, arnPrefix)
	props := f.store.Resource(trialComponentType, name)
	if props == nil {
		return nil, fault("ResourceNotFound")
	}
	if f.deleting[name] {
		delete(f.deleting, name)
		f.store.Remove(trialComponentType, name)
	}
	out := &sagemaker.DescribeTrialComponentOutput{}
	if err := awsapi.Decode(awsapi.Without(props, "Arn", "Id", "Tags"), out); err != nil {
		return nil, err
	}
	created := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	out.TrialComponentName = aws.String(name)
	out.TrialComponentArn = aws.String(arnPrefix + name)
	out.CreationTime, out.LastModifiedTime = &created, &created
	return out, nil
}

func (f *fakeAPI) UpdateTrialComponent(ctx context.Context, in *sagemaker.UpdateTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.UpdateTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	update := trialComponentValues{
		name:            in.TrialComponentName,
		displayName:     in.DisplayName,
		status:          in.Status,
		parameters:      in.Parameters,
		inputArtifacts:  in.InputArtifacts,
		outputArtifacts: in.OutputArtifacts,
	}
	if err := update.valid(); err != nil {
		return nil, err
	}
	for _, keys := range [][]string{in.ParametersToRemove, in.InputArtifactsToRemove, in.OutputArtifactsToRemove} {
		for _, key := range keys {
			if chars(key) > 256 {
				return nil, validationError
			}
		}
	}
	name := aws.ToString(in.TrialComponentName)
	return &sagemaker.UpdateTrialComponentOutput{}, f.change(func() error {
		props := f.store.Resource(trialComponentType, name)
		if props == nil {
			return fault("ResourceNotFound")
		}
		// The type has no hyperparameters or artifacts, so the fake doesn't keep them.
		in := *in
		in.Parameters, in.InputArtifacts, in.OutputArtifacts = nil, nil, nil
		in.ParametersToRemove, in.InputArtifactsToRemove, in.OutputArtifactsToRemove = nil, nil, nil
		values, err := awsapi.EncodeObject(in)
		if err != nil {
			return err
		}
		delete(values, "TrialComponentName")
		for k, v := range values {
			props[k] = v
		}
		f.store.Put(trialComponentType, name, props)
		return nil
	})
}

func (f *fakeAPI) DeleteTrialComponent(ctx context.Context, in *sagemaker.DeleteTrialComponentInput, _ ...func(*sagemaker.Options)) (*sagemaker.DeleteTrialComponentOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := aws.ToString(in.TrialComponentName)
	if !validName(name) {
		return nil, validationError
	}
	return &sagemaker.DeleteTrialComponentOutput{}, f.change(func() error {
		if f.store.Resource(trialComponentType, name) == nil || f.deleting[name] {
			return fault("ResourceNotFound")
		}
		f.deleting[name] = true
		return nil
	})
}

// ListTrialComponents returns one trial component per page.
func (f *fakeAPI) ListTrialComponents(ctx context.Context, in *sagemaker.ListTrialComponentsInput, _ ...func(*sagemaker.Options)) (*sagemaker.ListTrialComponentsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListTrialComponents"]; err != nil {
		return nil, err
	}
	if err := validListInput(in); err != nil {
		return nil, err
	}
	names := f.store.List(trialComponentType)
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	out := &sagemaker.ListTrialComponentsOutput{}
	if start < len(names) {
		out.TrialComponentSummaries = []types.TrialComponentSummary{{TrialComponentName: aws.String(names[start])}}
	}
	if start+1 < len(names) {
		out.NextToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}

// validListInput returns ValidationError, as SageMaker does, when a ListTrialComponents input is outside the limits
// SageMaker's API reference gives, or filters by more than one of ExperimentName, SourceArn, and TrialName.
func validListInput(in *sagemaker.ListTrialComponentsInput) error {
	filters := 0
	for _, filter := range []*string{in.ExperimentName, in.SourceArn, in.TrialName} {
		if filter != nil {
			filters++
		}
	}
	switch {
	case filters > 1,
		in.MaxResults != nil && (*in.MaxResults < 1 || *in.MaxResults > 100),
		chars(aws.ToString(in.NextToken)) > 8192,
		in.ExperimentName != nil && !validName(*in.ExperimentName),
		in.TrialName != nil && !validName(*in.TrialName),
		chars(aws.ToString(in.SourceArn)) > 256,
		in.SortBy != "" && !slices.Contains(in.SortBy.Values(), in.SortBy),
		in.SortOrder != "" && !slices.Contains(in.SortOrder.Values(), in.SortOrder):
		return validationError
	}
	return nil
}

func nameOf(arn *string) string {
	return strings.TrimPrefix(aws.ToString(arn), arnPrefix)
}

// tagsOf returns the stored tags of the trial component arn names.
func (f *fakeAPI) tagsOf(arn *string) (map[string]any, map[string]string, error) {
	props := f.store.Resource(trialComponentType, nameOf(arn))
	if props == nil {
		return nil, nil, fault("ResourceNotFound")
	}
	tags := map[string]string{}
	for _, t := range awsapi.Tags(props) {
		tags[t.Key] = t.Value
	}
	return props, tags, nil
}

func (f *fakeAPI) putTags(arn *string, props map[string]any, tags map[string]string) {
	if list := awsapi.TagList(tags); list != nil {
		props["Tags"] = list
	} else {
		delete(props, "Tags")
	}
	f.store.Put(trialComponentType, nameOf(arn), props)
}

func (f *fakeAPI) AddTags(ctx context.Context, in *sagemaker.AddTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.AddTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := validResourceArn(in.ResourceArn); err != nil {
		return nil, err
	}
	if err := validTags(in.Tags); err != nil {
		return nil, err
	}
	return &sagemaker.AddTagsOutput{}, f.change(func() error {
		props, tags, err := f.tagsOf(in.ResourceArn)
		if err != nil {
			return err
		}
		for _, t := range in.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		f.putTags(in.ResourceArn, props, tags)
		return nil
	})
}

func (f *fakeAPI) DeleteTags(ctx context.Context, in *sagemaker.DeleteTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.DeleteTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := validResourceArn(in.ResourceArn); err != nil {
		return nil, err
	}
	if len(in.TagKeys) < 1 || len(in.TagKeys) > 50 {
		return nil, validationError
	}
	for _, key := range in.TagKeys {
		if chars(key) < 1 || chars(key) > 128 || !tagPattern.MatchString(key) {
			return nil, validationError
		}
	}
	return &sagemaker.DeleteTagsOutput{}, f.change(func() error {
		props, tags, err := f.tagsOf(in.ResourceArn)
		if err != nil {
			return err
		}
		for key := range tags {
			if slices.Contains(in.TagKeys, key) {
				delete(tags, key)
			}
		}
		f.putTags(in.ResourceArn, props, tags)
		return nil
	})
}

// ListTags returns one tag per page.
func (f *fakeAPI) ListTags(ctx context.Context, in *sagemaker.ListTagsInput, _ ...func(*sagemaker.Options)) (*sagemaker.ListTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.errs["ListTags"]; err != nil {
		return nil, err
	}
	if err := validResourceArn(in.ResourceArn); err != nil {
		return nil, err
	}
	if (in.MaxResults != nil && *in.MaxResults < 50) || chars(aws.ToString(in.NextToken)) > 8192 {
		return nil, validationError
	}
	_, tags, err := f.tagsOf(in.ResourceArn)
	if err != nil {
		return nil, err
	}
	list := awsapi.Tags(map[string]any{"Tags": awsapi.TagList(tags)})
	start, _ := strconv.Atoi(aws.ToString(in.NextToken))
	out := &sagemaker.ListTagsOutput{}
	if start < len(list) {
		out.Tags = []types.Tag{{Key: aws.String(list[start].Key), Value: aws.String(list[start].Value)}}
	}
	if start+1 < len(list) {
		out.NextToken = aws.String(strconv.Itoa(start + 1))
	}
	return out, nil
}
