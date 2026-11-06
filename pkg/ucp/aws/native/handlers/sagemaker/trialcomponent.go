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

// Package sagemaker has the native handler for Amazon SageMaker experiment trial components, which Cloud Control
// can't create in any region.
package sagemaker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/aws/aws-sdk-go-v2/service/sagemaker/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const trialComponentType = "AWS::SageMaker::ExperimentTrialComponent"

// API is the part of the SageMaker client the handler uses.
type API interface {
	CreateTrialComponent(ctx context.Context, params *sagemaker.CreateTrialComponentInput, optFns ...func(*sagemaker.Options)) (*sagemaker.CreateTrialComponentOutput, error)
	DescribeTrialComponent(ctx context.Context, params *sagemaker.DescribeTrialComponentInput, optFns ...func(*sagemaker.Options)) (*sagemaker.DescribeTrialComponentOutput, error)
	UpdateTrialComponent(ctx context.Context, params *sagemaker.UpdateTrialComponentInput, optFns ...func(*sagemaker.Options)) (*sagemaker.UpdateTrialComponentOutput, error)
	DeleteTrialComponent(ctx context.Context, params *sagemaker.DeleteTrialComponentInput, optFns ...func(*sagemaker.Options)) (*sagemaker.DeleteTrialComponentOutput, error)
	ListTrialComponents(ctx context.Context, params *sagemaker.ListTrialComponentsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListTrialComponentsOutput, error)
	AddTags(ctx context.Context, params *sagemaker.AddTagsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.AddTagsOutput, error)
	DeleteTags(ctx context.Context, params *sagemaker.DeleteTagsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.DeleteTagsOutput, error)
	ListTags(ctx context.Context, params *sagemaker.ListTagsInput, optFns ...func(*sagemaker.Options)) (*sagemaker.ListTagsOutput, error)
}

var inRegion = awsapi.InRegion[sagemaker.Options]

var (
	_ native.Handler           = (*TrialComponent)(nil)
	_ native.IdentifierEncoder = (*TrialComponent)(nil)
)

// timeProperties are the properties that hold a date and time.
var timeProperties = []string{"StartTime", "EndTime"}

// TrialComponent serves AWS::SageMaker::ExperimentTrialComponent. Its identifier, Id, is TrialComponentName, which the
// template must set. Every call takes effect at once.
type TrialComponent struct {
	api API
}

// NewTrialComponent returns the handler for SageMaker experiment trial components.
func NewTrialComponent(api API) *TrialComponent {
	return &TrialComponent{api: api}
}

// TypeName returns AWS::SageMaker::ExperimentTrialComponent.
func (h *TrialComponent) TypeName() string {
	return trialComponentType
}

// Options returns how UCP uses the handler.
func (h *TrialComponent) Options() native.Options {
	return native.Options{
		List:       native.ListRegion,
		Recovery:   native.RecoverByIdentifier,
		Equivalent: equivalent,
	}
}

// Identifier returns TrialComponentName.
func (h *TrialComponent) Identifier(properties map[string]any) (string, error) {
	return awsapi.String(properties, "TrialComponentName"), nil
}

// equivalent treats a date and time AWS returns in another form, such as in UTC or with milliseconds, as the
// template's value when both name the same instant.
func equivalent(path string, template any, aws any) bool {
	if !slices.Contains(timeProperties, path) {
		return false
	}
	t, okT := template.(string)
	a, okA := aws.(string)
	if !okT || !okA {
		return false
	}
	tt, errT := time.Parse(time.RFC3339Nano, t)
	at, errA := time.Parse(time.RFC3339Nano, a)
	return errT == nil && errA == nil && tt.Equal(at)
}

func (h *TrialComponent) describe(ctx context.Context, region string, name string) (*sagemaker.DescribeTrialComponentOutput, error) {
	out, err := h.api.DescribeTrialComponent(ctx, &sagemaker.DescribeTrialComponentInput{TrialComponentName: aws.String(name)}, inRegion(region))
	if err != nil {
		return nil, awsapi.NotFound(err, "ResourceNotFound")
	}
	return out, nil
}

// Read returns the trial component's properties.
func (h *TrialComponent) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	out, err := h.describe(ctx, region, identifier)
	if err != nil {
		return nil, err
	}
	return h.properties(ctx, region, out)
}

func (h *TrialComponent) properties(ctx context.Context, region string, c *sagemaker.DescribeTrialComponentOutput) (map[string]any, error) {
	tags := map[string]string{}
	var token *string
	for {
		page, err := h.api.ListTags(ctx, &sagemaker.ListTagsInput{ResourceArn: c.TrialComponentArn, NextToken: token}, inRegion(region))
		if err != nil {
			return nil, awsapi.NotFound(err, "ResourceNotFound")
		}
		for _, t := range page.Tags {
			tags[aws.ToString(t.Key)] = aws.ToString(t.Value)
		}
		if token = page.NextToken; token == nil {
			break
		}
	}
	return awsapi.EncodeObject(map[string]any{
		"Id":                 c.TrialComponentName,
		"TrialComponentName": c.TrialComponentName,
		"Arn":                c.TrialComponentArn,
		"DisplayName":        c.DisplayName,
		"StartTime":          timeString(c.StartTime),
		"EndTime":            timeString(c.EndTime),
		"Status":             c.Status,
		"MetadataProperties": c.MetadataProperties,
		"LineageGroupArn":    c.LineageGroupArn,
		"CreationTime":       timeString(c.CreationTime),
		"LastModifiedTime":   timeString(c.LastModifiedTime),
		"Tags":               awsapi.TagList(tags),
	})
}

// timeString returns t in RFC 3339 format in UTC, or nil when t is nil.
func timeString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// timeOf returns the date and time property name of properties, or nil when it isn't set.
func timeOf(properties map[string]any, name string) (*time.Time, error) {
	s := awsapi.String(properties, name)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, &native.Failure{
			Message:     fmt.Sprintf("%s must be a date and time in RFC 3339 format, such as 2026-01-02T15:04:05Z.", name),
			ClientError: true,
			Outcome:     native.OutcomeRejected,
		}
	}
	return &t, nil
}

// Create creates the trial component.
func (h *TrialComponent) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	p := request.Properties
	input := &sagemaker.CreateTrialComponentInput{}
	if err := awsapi.Decode(awsapi.Without(p, append([]string{"Tags"}, timeProperties...)...), input); err != nil {
		return "", err
	}
	var err error
	if input.StartTime, err = timeOf(p, "StartTime"); err != nil {
		return "", err
	}
	if input.EndTime, err = timeOf(p, "EndTime"); err != nil {
		return "", err
	}
	input.Tags = awsapi.SDKTags[types.Tag](awsapi.Tags(p))
	if _, err := h.api.CreateTrialComponent(ctx, input, inRegion(request.Region)); err != nil {
		return "", err
	}
	return aws.ToString(input.TrialComponentName), nil
}

// Update makes the patch's changes with UpdateTrialComponent, then adds and removes tags. UpdateTrialComponent can't
// clear a value, so a property the template removes keeps AWS's value.
func (h *TrialComponent) Update(ctx context.Context, request native.UpdateRequest) error {
	current, err := h.describe(ctx, request.Region, request.Identifier)
	if err != nil {
		return err
	}
	input, err := updateInput(request)
	if err != nil {
		return err
	}
	opt := inRegion(request.Region)
	calls := []func() error{}
	if input != nil {
		calls = append(calls, func() error {
			_, err := h.api.UpdateTrialComponent(ctx, input, opt)
			return err
		})
	}
	if slices.Contains(awsapi.Changed(request), "Tags") {
		calls = append(calls, h.tagCalls(ctx, current.TrialComponentArn, request, opt)...)
	}
	return awsapi.Calls(calls...)
}

// updateInput returns the UpdateTrialComponent input for the request's changes, or nil when none of them needs it.
func updateInput(request native.UpdateRequest) (*sagemaker.UpdateTrialComponentInput, error) {
	changed := awsapi.Changed(request)
	input := &sagemaker.UpdateTrialComponentInput{TrialComponentName: aws.String(request.Identifier)}
	update := false
	if name := awsapi.String(request.Desired, "DisplayName"); name != "" && slices.Contains(changed, "DisplayName") {
		input.DisplayName, update = aws.String(name), true
	}
	if status, ok := request.Desired["Status"].(map[string]any); ok && slices.Contains(changed, "Status") {
		input.Status = &types.TrialComponentStatus{}
		if err := awsapi.Decode(status, input.Status); err != nil {
			return nil, err
		}
		update = true
	}
	for _, name := range timeProperties {
		if !slices.Contains(changed, name) {
			continue
		}
		t, err := timeOf(request.Desired, name)
		if err != nil {
			return nil, err
		}
		switch {
		case t == nil:
			continue
		case name == "StartTime":
			input.StartTime = t
		default:
			input.EndTime = t
		}
		update = true
	}
	if !update {
		return nil, nil
	}
	return input, nil
}

// tagCalls returns the calls that add and remove tags to go from the request's current tags to the desired ones.
func (h *TrialComponent) tagCalls(ctx context.Context, arn *string, request native.UpdateRequest, opt func(*sagemaker.Options)) []func() error {
	set, remove := awsapi.TagChanges(awsapi.Tags(request.Current), awsapi.Tags(request.Desired))
	calls := []func() error{}
	if len(set) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.AddTags(ctx, &sagemaker.AddTagsInput{ResourceArn: arn, Tags: awsapi.SDKTags[types.Tag](set)}, opt)
			return err
		})
	}
	if len(remove) > 0 {
		calls = append(calls, func() error {
			_, err := h.api.DeleteTags(ctx, &sagemaker.DeleteTagsInput{ResourceArn: arn, TagKeys: remove}, opt)
			return err
		})
	}
	return calls
}

// Delete deletes the trial component.
func (h *TrialComponent) Delete(ctx context.Context, region string, identifier string) error {
	_, err := h.api.DeleteTrialComponent(ctx, &sagemaker.DeleteTrialComponentInput{TrialComponentName: aws.String(identifier)}, inRegion(region))
	return awsapi.NotFound(err, "ResourceNotFound")
}

// List returns every trial component in the region, including those SageMaker made for its jobs.
func (h *TrialComponent) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var token *string
	for {
		page, err := h.api.ListTrialComponents(ctx, &sagemaker.ListTrialComponentsInput{NextToken: token}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, summary := range page.TrialComponentSummaries {
			name := aws.ToString(summary.TrialComponentName)
			properties, err := h.Read(ctx, region, name)
			if errors.Is(err, native.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out = append(out, native.Resource{Identifier: name, Properties: properties})
		}
		if token = page.NextToken; token == nil {
			return out, nil
		}
	}
}

// Progress reports a create or update as done, because both take effect at once. A delete waits until AWS no longer
// has the trial component.
func (h *TrialComponent) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	if op != native.OperationDelete {
		return native.Progress{Done: true}, nil
	}
	if _, err := h.describe(ctx, region, identifier); err != nil {
		return native.Progress{}, err
	}
	return native.Progress{State: "deleting", Waiting: "deleted"}, nil
}
