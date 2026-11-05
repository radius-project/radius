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
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"

	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsreplay"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

// Test_Replay_LoggingOption adds a CloudWatch logging option to an application, checks that AWS rejects a second one
// with the same log stream, moves it to another log stream, and removes it, against AWS's recorded responses.
func Test_Replay_LoggingOption(t *testing.T) {
	s := awsreplay.New(t, "testdata/loggingoption.json")
	api := kinesisanalyticsv2.NewFromConfig(s.Config(t.Context()))
	h := NewLoggingOption(api)
	application := s.Input("RADIUS_REC_KDA_APPLICATION")

	properties := loggingOption(application, s.Input("RADIUS_REC_KDA_LOG_STREAM_ARN"))
	nativetest.Lifecycle{
		Handler:    h,
		Session:    s,
		Properties: properties,
		Update:     loggingOption(application, s.Input("RADIUS_REC_KDA_LOG_STREAM_ARN_2")),
		Poll:       10 * time.Second,
		Between: func() {
			requireDuplicateRejected(t, s, h, api, properties)
		},
	}.Run(t)
}

// requireDuplicateRejected adds a second logging option with the log stream of properties, which the application
// already has. Find relies on AWS rejecting it, or keeping one option for the log stream. When AWS adds a second
// option, the type can't be served, and the test removes the extra options and fails.
func requireDuplicateRejected(t *testing.T, s *awsreplay.Session, h *LoggingOption, api *kinesisanalyticsv2.Client, properties map[string]any) {
	ctx := t.Context()
	region := awsreplay.Region
	_, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties, ClientToken: "22222222-3333-4444-5555-666666666666"})
	var failure *native.Failure
	switch {
	case err == nil:
		// AWS kept one logging option for the log stream, so Find still finds exactly one.
		h.waitReady(t, s, awsapi.String(properties, "ApplicationName"))
		return
	case !errors.As(err, &failure):
		return
	}
	t.Errorf("AWS accepted a second CloudWatch logging option with the same log stream, so UCP can't find a lost create")
	name := awsapi.String(properties, "ApplicationName")
	app, err := h.describe(ctx, region, name)
	if err != nil {
		t.Fatalf("failed to read the application to remove the extra logging options: %v", err)
	}
	ids := withLogStream(app.CloudWatchLoggingOptionDescriptions, logStream(properties))
	for _, id := range ids[1:] {
		h.waitReady(t, s, name)
		app, err = h.describe(ctx, region, name)
		if err != nil {
			t.Fatalf("failed to read the application to remove the extra logging options: %v", err)
		}
		_, err = api.DeleteApplicationCloudWatchLoggingOption(ctx, &kinesisanalyticsv2.DeleteApplicationCloudWatchLoggingOptionInput{
			ApplicationName:             aws.String(name),
			CloudWatchLoggingOptionId:   aws.String(id),
			CurrentApplicationVersionId: app.ApplicationVersionId,
		}, inRegion(region))
		if err != nil {
			t.Fatalf("failed to remove the extra logging option %s: %v", id, err)
		}
	}
	h.waitReady(t, s, name)
}

// waitReady waits until the application is no longer updating.
func (h *LoggingOption) waitReady(t *testing.T, s *awsreplay.Session, name string) {
	for range 60 {
		app, err := h.describe(t.Context(), awsreplay.Region, name)
		if err != nil {
			t.Fatalf("failed to read the application: %v", err)
		}
		if applicationStates.Progress(string(app.ApplicationStatus)).Done {
			return
		}
		s.Wait(10 * time.Second)
	}
	t.Fatalf("the application kept updating")
}
