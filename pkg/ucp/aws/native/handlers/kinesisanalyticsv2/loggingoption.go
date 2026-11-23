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

// Package kinesisanalyticsv2 has the native handler for the CloudWatch logging options of Amazon Managed Service for
// Apache Flink applications (Kinesis Data Analytics v2), which Cloud Control can't create in any region.
package kinesisanalyticsv2

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2/types"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
)

const loggingOptionType = "AWS::KinesisAnalyticsV2::ApplicationCloudWatchLoggingOption"

// API is the part of the Kinesis Analytics v2 client the handler uses.
type API interface {
	DescribeApplication(ctx context.Context, params *kinesisanalyticsv2.DescribeApplicationInput, optFns ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.DescribeApplicationOutput, error)
	ListApplications(ctx context.Context, params *kinesisanalyticsv2.ListApplicationsInput, optFns ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.ListApplicationsOutput, error)
	AddApplicationCloudWatchLoggingOption(ctx context.Context, params *kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionInput, optFns ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionOutput, error)
	DeleteApplicationCloudWatchLoggingOption(ctx context.Context, params *kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionInput, optFns ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionOutput, error)
	UpdateApplication(ctx context.Context, params *kinesisanalyticsv2.UpdateApplicationInput, optFns ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.UpdateApplicationOutput, error)
}

var inRegion = awsapi.InRegion[kinesisanalyticsv2.Options]

var (
	_ native.Handler           = (*LoggingOption)(nil)
	_ native.IdentifierEncoder = (*LoggingOption)(nil)
	_ native.Finder            = (*LoggingOption)(nil)
)

// applicationStates are the application statuses a change to a logging option waits for. AWS updates the
// application, which reports UPDATING until it is READY again, or RUNNING for an application that runs.
var applicationStates = awsapi.States{Done: []string{string(types.ApplicationStatusReady), string(types.ApplicationStatusRunning)}}

// LoggingOption serves AWS::KinesisAnalyticsV2::ApplicationCloudWatchLoggingOption. Its identifier, Id, is
// ApplicationName and the CloudWatchLoggingOptionId AWS assigns, joined with "|". The API takes no idempotency token,
// so a lost create is found by its log stream, which UCP expects AWS to keep unique within an application.
type LoggingOption struct {
	api API
}

// NewLoggingOption returns the handler for the CloudWatch logging options of applications.
func NewLoggingOption(api API) *LoggingOption {
	return &LoggingOption{api: api}
}

// TypeName returns AWS::KinesisAnalyticsV2::ApplicationCloudWatchLoggingOption.
func (h *LoggingOption) TypeName() string {
	return loggingOptionType
}

// Options returns how UCP uses the handler.
func (h *LoggingOption) Options() native.Options {
	return native.Options{
		List:     native.ListRegion,
		Recovery: native.RecoverBySearch,
	}
}

// Identifier returns Id, or "" before AWS assigns the logging option's ID.
func (h *LoggingOption) Identifier(properties map[string]any) (string, error) {
	return awsapi.String(properties, "Id"), nil
}

// parse returns the application name and the logging option ID of an identifier.
func parse(identifier string) (string, string, error) {
	application, option, ok := strings.Cut(identifier, "|")
	if !ok || application == "" || option == "" {
		return "", "", fmt.Errorf("the AWS identifier %q is not an application name and a CloudWatch logging option ID joined with \"|\"", identifier)
	}
	return application, option, nil
}

// describe returns the application, with AWS's error when it fails.
func (h *LoggingOption) describe(ctx context.Context, region string, name string) (*types.ApplicationDetail, error) {
	out, err := h.api.DescribeApplication(ctx, &kinesisanalyticsv2.DescribeApplicationInput{ApplicationName: aws.String(name)}, inRegion(region))
	if err != nil {
		return nil, err
	}
	if out.ApplicationDetail == nil {
		return nil, errors.New("AWS described the application without its details")
	}
	return out.ApplicationDetail, nil
}

// application returns the application, or ErrNotFound when AWS has none or is deleting it.
func (h *LoggingOption) application(ctx context.Context, region string, name string) (*types.ApplicationDetail, error) {
	app, err := h.describe(ctx, region, name)
	if err != nil {
		return nil, awsapi.NotFound(err, "ResourceNotFoundException")
	}
	if app.ApplicationStatus == types.ApplicationStatusDeleting {
		return nil, native.ErrNotFound
	}
	return app, nil
}

// option returns the application's logging option with the ID, or nil.
func option(app *types.ApplicationDetail, id string) *types.CloudWatchLoggingOptionDescription {
	for i := range app.CloudWatchLoggingOptionDescriptions {
		if o := &app.CloudWatchLoggingOptionDescriptions[i]; aws.ToString(o.CloudWatchLoggingOptionId) == id {
			return o
		}
	}
	return nil
}

// withLogStream returns the IDs of the logging options in options that send to logStream.
func withLogStream(options []types.CloudWatchLoggingOptionDescription, logStream string) []string {
	out := []string{}
	for _, o := range options {
		if aws.ToString(o.LogStreamARN) == logStream {
			out = append(out, aws.ToString(o.CloudWatchLoggingOptionId))
		}
	}
	return out
}

// logStream returns the template's CloudWatchLoggingOption.LogStreamARN.
func logStream(properties map[string]any) string {
	o, _ := properties["CloudWatchLoggingOption"].(map[string]any)
	return awsapi.String(o, "LogStreamARN")
}

func properties(app *types.ApplicationDetail, o *types.CloudWatchLoggingOptionDescription) map[string]any {
	name := aws.ToString(app.ApplicationName)
	return map[string]any{
		"Id":                      name + "|" + aws.ToString(o.CloudWatchLoggingOptionId),
		"ApplicationName":         name,
		"CloudWatchLoggingOption": map[string]any{"LogStreamARN": aws.ToString(o.LogStreamARN)},
	}
}

// Read returns the logging option's properties.
func (h *LoggingOption) Read(ctx context.Context, region string, identifier string) (map[string]any, error) {
	name, id, err := parse(identifier)
	if err != nil {
		return nil, err
	}
	app, err := h.application(ctx, region, name)
	if err != nil {
		return nil, err
	}
	o := option(app, id)
	if o == nil {
		return nil, native.ErrNotFound
	}
	return properties(app, o), nil
}

// Create adds the logging option to the application, and returns its identifier from AWS's answer.
func (h *LoggingOption) Create(ctx context.Context, request native.CreateRequest) (string, error) {
	name := awsapi.String(request.Properties, "ApplicationName")
	app, err := h.describe(ctx, request.Region, name)
	if err != nil {
		return "", err
	}
	stream := logStream(request.Properties)
	out, err := h.api.AddApplicationCloudWatchLoggingOption(ctx, &kinesisanalyticsv2.AddApplicationCloudWatchLoggingOptionInput{
		ApplicationName:             aws.String(name),
		CloudWatchLoggingOption:     &types.CloudWatchLoggingOption{LogStreamARN: aws.String(stream)},
		CurrentApplicationVersionId: app.ApplicationVersionId,
	}, inRegion(request.Region))
	if err != nil {
		return "", err
	}
	ids := withLogStream(out.CloudWatchLoggingOptionDescriptions, stream)
	if len(ids) != 1 {
		return "", &native.Failure{
			Message: fmt.Sprintf("AWS added the CloudWatch logging option, but the application has %d logging options with its LogStreamARN, so UCP can't tell which one it added.", len(ids)),
			Outcome: native.OutcomeApplied,
		}
	}
	return name + "|" + ids[0], nil
}

// Find returns the identifier of the application's logging option with the template's log stream, or "" when the
// application has none.
func (h *LoggingOption) Find(ctx context.Context, request native.CreateRequest) (string, error) {
	name := awsapi.String(request.Properties, "ApplicationName")
	app, err := h.application(ctx, request.Region, name)
	if errors.Is(err, native.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	ids := withLogStream(app.CloudWatchLoggingOptionDescriptions, logStream(request.Properties))
	switch len(ids) {
	case 0:
		return "", nil
	case 1:
		return name + "|" + ids[0], nil
	}
	return "", awsapi.Ambiguous("CloudWatch logging options", ids)
}

// Update sends the template's log stream with UpdateApplication.
func (h *LoggingOption) Update(ctx context.Context, request native.UpdateRequest) error {
	if !slices.Contains(awsapi.Changed(request), "CloudWatchLoggingOption") {
		return nil
	}
	name, id, err := parse(request.Identifier)
	if err != nil {
		return err
	}
	app, err := h.application(ctx, request.Region, name)
	if err != nil {
		return err
	}
	if option(app, id) == nil {
		return native.ErrNotFound
	}
	_, err = h.api.UpdateApplication(ctx, &kinesisanalyticsv2.UpdateApplicationInput{
		ApplicationName:             aws.String(name),
		CurrentApplicationVersionId: app.ApplicationVersionId,
		CloudWatchLoggingOptionUpdates: []types.CloudWatchLoggingOptionUpdate{{
			CloudWatchLoggingOptionId: aws.String(id),
			LogStreamARNUpdate:        aws.String(logStream(request.Desired)),
		}},
	}, inRegion(request.Region))
	return err
}

// Delete removes the logging option from the application.
func (h *LoggingOption) Delete(ctx context.Context, region string, identifier string) error {
	name, id, err := parse(identifier)
	if err != nil {
		return err
	}
	app, err := h.application(ctx, region, name)
	if err != nil {
		return err
	}
	if option(app, id) == nil {
		return native.ErrNotFound
	}
	_, err = h.api.DeleteApplicationCloudWatchLoggingOption(ctx, &kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionInput{
		ApplicationName:             aws.String(name),
		CloudWatchLoggingOptionId:   aws.String(id),
		CurrentApplicationVersionId: app.ApplicationVersionId,
	}, inRegion(region))
	return awsapi.NotFound(err, "ResourceNotFoundException")
}

// List returns the logging options of every application in the region.
func (h *LoggingOption) List(ctx context.Context, region string) ([]native.Resource, error) {
	out := []native.Resource{}
	var token *string
	for {
		page, err := h.api.ListApplications(ctx, &kinesisanalyticsv2.ListApplicationsInput{NextToken: token}, inRegion(region))
		if err != nil {
			return nil, err
		}
		for _, summary := range page.ApplicationSummaries {
			app, err := h.application(ctx, region, aws.ToString(summary.ApplicationName))
			if errors.Is(err, native.ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for i := range app.CloudWatchLoggingOptionDescriptions {
				p := properties(app, &app.CloudWatchLoggingOptionDescriptions[i])
				out = append(out, native.Resource{Identifier: p["Id"].(string), Properties: p})
			}
		}
		if token = page.NextToken; token == nil {
			return out, nil
		}
	}
}

// Progress reads the application's status, which AWS sets to UPDATING while it applies a change to a logging option.
// A delete finishes once the logging option is gone and the application is no longer updating.
func (h *LoggingOption) Progress(ctx context.Context, region string, identifier string, op native.Operation) (native.Progress, error) {
	name, id, err := parse(identifier)
	if err != nil {
		return native.Progress{}, err
	}
	app, err := h.application(ctx, region, name)
	if err != nil {
		return native.Progress{}, err
	}
	status := string(app.ApplicationStatus)
	present := option(app, id) != nil
	if op == native.OperationDelete {
		if !present && app.ApplicationStatus != types.ApplicationStatusUpdating {
			return native.Progress{}, native.ErrNotFound
		}
		return native.Progress{State: status, Waiting: "deleted"}, nil
	}
	if !present {
		return native.Progress{}, native.ErrNotFound
	}
	return applicationStates.Progress(status), nil
}
