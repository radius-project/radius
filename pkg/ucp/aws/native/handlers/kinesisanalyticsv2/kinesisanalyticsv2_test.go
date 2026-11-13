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
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2"
	"github.com/aws/aws-sdk-go-v2/service/kinesisanalyticsv2/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region    = "us-west-2"
	streamOne = "arn:aws:logs:us-west-2:123456789012:log-group:app:log-stream:one"
	streamTwo = "arn:aws:logs:us-west-2:123456789012:log-group:app:log-stream:two"
)

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

func loggingOption(application string, stream string) map[string]any {
	return map[string]any{
		"ApplicationName":         application,
		"CloudWatchLoggingOption": map[string]any{"LogStreamARN": stream},
	}
}

func Test_Suite_LoggingOption(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		NoCloudControl: true,
		TypeName:       loggingOptionType,
		Schema:         schema(t, "aws-kinesisanalyticsv2-applicationcloudwatchloggingoption.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewLoggingOption(newFakeAPI(store))
		},
		IdentifierProperty: "Id",
		Properties:         loggingOption("app-one", streamOne),
		Changed:            loggingOption("app-one", streamTwo),
		CreateOnlyChange:   loggingOption("app-two", streamOne),
		API:                loggingOptionAPI,
	})
}

func newHandler() (*LoggingOption, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewLoggingOption(api), api
}

func create(t *testing.T, h *LoggingOption, properties map[string]any) string {
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
	require.NoError(t, err)
	return id
}

func Test_InRegion(t *testing.T) {
	opts := kinesisanalyticsv2.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Identifier(loggingOption("app-one", streamOne))
	require.NoError(t, err)
	require.Empty(t, id, "AWS assigns the logging option's ID")

	id, err = h.Identifier(map[string]any{"Id": "app-one|1.1"})
	require.NoError(t, err)
	require.Equal(t, "app-one|1.1", id)
}

func Test_InvalidIdentifier(t *testing.T) {
	h, _ := newHandler()
	for _, id := range []string{"app-one", "|1.1", "app-one|"} {
		_, err := h.Read(t.Context(), region, id)
		require.ErrorContains(t, err, "is not an application name and a CloudWatch logging option ID", id)
		require.ErrorContains(t, h.Delete(t.Context(), region, id), "is not an application name")
		_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
		require.ErrorContains(t, err, "is not an application name")
		err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: id, Patch: replace("CloudWatchLoggingOption")})
		require.ErrorContains(t, err, "is not an application name")
	}
}

func Test_Create(t *testing.T) {
	h, api := newHandler()
	id := create(t, h, loggingOption("app-one", streamOne))
	require.Equal(t, "app-one|1.1", id)
	require.Equal(t, int64(2), api.apps["app-one"].version, "the add sent the application's current version")

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("missing", streamOne)})
	require.ErrorContains(t, err, "ResourceNotFoundException")
	require.NotErrorIs(t, err, native.ErrNotFound, "AWS's error is returned as it is")

	api.errs["AddApplicationCloudWatchLoggingOption"] = fault("InvalidArgumentException")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("app-one", streamTwo)})
	require.ErrorContains(t, err, "InvalidArgumentException")
}

// Test_Create_Duplicate adds a second logging option with the same log stream, which UCP expects AWS to reject. When
// AWS accepts it, the handler can't tell which option it added, and says so without naming the log stream.
func Test_Create_Duplicate(t *testing.T) {
	h, _ := newHandler()
	create(t, h, loggingOption("app-one", streamOne))

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("app-one", streamOne)})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeApplied, failure.Outcome)
	require.NotContains(t, failure.Message, streamOne)

	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("app-one", streamOne)})
	require.ErrorContains(t, err, "2 CloudWatch logging options")
	require.NotContains(t, err.Error(), streamOne)
}

func Test_Find(t *testing.T) {
	h, api := newHandler()
	id, err := h.Find(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("app-one", streamOne)})
	require.NoError(t, err)
	require.Empty(t, id, "the application has no logging option")

	id, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("missing", streamOne)})
	require.NoError(t, err)
	require.Empty(t, id, "AWS has no such application")

	want := create(t, h, loggingOption("app-one", streamOne))
	create(t, h, loggingOption("app-one", streamTwo))
	id, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("app-one", streamOne)})
	require.NoError(t, err)
	require.Equal(t, want, id)

	api.errs["DescribeApplication"] = errors.New("boom")
	_, err = h.Find(t.Context(), native.CreateRequest{Region: region, Properties: loggingOption("app-one", streamOne)})
	require.EqualError(t, err, "boom", "UCP can't tell, so it doesn't create again")
}

func replace(properties ...string) jsondiff.Patch {
	patch := jsondiff.Patch{}
	for _, p := range properties {
		patch = append(patch, jsondiff.Operation{Type: "replace", Path: "/" + p})
	}
	return patch
}

func Test_Update(t *testing.T) {
	h, api := newHandler()
	id := create(t, h, loggingOption("app-one", streamOne))
	update := func(identifier string, desired map[string]any, patch jsondiff.Patch) error {
		return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: identifier, Desired: desired, Patch: patch})
	}

	require.NoError(t, update(id, loggingOption("app-one", streamOne), nil), "nothing changed")
	require.Equal(t, int64(2), api.apps["app-one"].version)

	require.NoError(t, update(id, loggingOption("app-one", streamTwo), replace("CloudWatchLoggingOption")))
	props, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, loggingOption("app-one", streamTwo)["CloudWatchLoggingOption"], props["CloudWatchLoggingOption"])

	require.ErrorIs(t, update("app-one|9.1", loggingOption("app-one", streamTwo), replace("CloudWatchLoggingOption")), native.ErrNotFound)
	require.ErrorIs(t, update("missing|1.1", loggingOption("missing", streamTwo), replace("CloudWatchLoggingOption")), native.ErrNotFound)
}

func Test_Delete(t *testing.T) {
	h, api := newHandler()
	require.ErrorIs(t, h.Delete(t.Context(), region, "app-one|1.1"), native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), region, "missing|1.1"), native.ErrNotFound)

	id := create(t, h, loggingOption("app-one", streamOne))
	api.errs["DeleteApplicationCloudWatchLoggingOption"] = fault("ResourceNotFoundException")
	require.ErrorIs(t, h.Delete(t.Context(), region, id), native.ErrNotFound)
}

func Test_Progress(t *testing.T) {
	h, api := newHandler()
	id := create(t, h, loggingOption("app-two", streamOne))

	progress, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "UPDATING", Waiting: "READY"}, progress)
	progress, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Done: true, State: "RUNNING"}, progress, "a running application is done")

	_, err = h.Progress(t.Context(), region, "app-two|9.1", native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)

	require.NoError(t, h.Delete(t.Context(), region, id))
	progress, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "UPDATING", Waiting: "deleted"}, progress, "the application is still updating")
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Progress(t.Context(), region, "missing|1.1", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	api.apps["app-two"].status = types.ApplicationStatusDeleting
	_, err = h.Read(t.Context(), region, "app-two|1.1")
	require.ErrorIs(t, err, native.ErrNotFound, "an application AWS is deleting has no logging options")
}

func Test_List(t *testing.T) {
	h, api := newHandler()
	one := create(t, h, loggingOption("app-one", streamOne))
	two := create(t, h, loggingOption("app-two", streamTwo))
	api.apps["app-deleting"] = &application{status: types.ApplicationStatusDeleting}

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Equal(t, []native.Resource{
		{Identifier: one, Properties: map[string]any{"Id": one, "ApplicationName": "app-one", "CloudWatchLoggingOption": map[string]any{"LogStreamARN": streamOne}}},
		{Identifier: two, Properties: map[string]any{"Id": two, "ApplicationName": "app-two", "CloudWatchLoggingOption": map[string]any{"LogStreamARN": streamTwo}}},
	}, resources)

	api.errs["DescribeApplication"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")

	api.errs["ListApplications"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

// noDetail is an API whose DescribeApplication returns no application details.
type noDetail struct {
	*fakeAPI
}

func (n noDetail) DescribeApplication(ctx context.Context, in *kinesisanalyticsv2.DescribeApplicationInput, _ ...func(*kinesisanalyticsv2.Options)) (*kinesisanalyticsv2.DescribeApplicationOutput, error) {
	return &kinesisanalyticsv2.DescribeApplicationOutput{}, nil
}

func Test_Read_NoDetail(t *testing.T) {
	h := NewLoggingOption(noDetail{newFakeAPI(awsfake.NewCloudControl())})
	_, err := h.Read(t.Context(), region, "app-one|1.1")
	require.EqualError(t, err, "AWS described the application without its details")
}
