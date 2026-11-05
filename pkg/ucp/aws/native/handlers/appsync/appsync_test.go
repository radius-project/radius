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

package appsync

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region = "us-west-2"
	apiID  = "abcdefghijklmnopqrstuvwxyz"
)

func read(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

func Test_Suite_GraphQLSchema(t *testing.T) {
	properties := map[string]any{"ApiId": apiID, "Definition": "type Query { hello: String }"}
	nativetest.Run(t, nativetest.Suite{
		TypeName: schemaType,
		Schema:   read(t, "aws-appsync-graphqlschema.json"),
		Metadata: read(t, "aws-appsync-graphqlschema.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store, apiID)
			return NewGraphQLSchema(api, api)
		},
		IdentifierProperty: "ApiId",
		WriteOnly:          []string{"Definition", "DefinitionS3Location"},
		Properties:         properties,
		Identifier:         apiID,
		Changed:            map[string]any{"ApiId": apiID, "Definition": "type Query { hello: String, bye: String }"},
		ChangedWriteOnly:   map[string]any{"ApiId": apiID, "Definition": "type Query { hi: String }"},
		CreateOnlyChange:   map[string]any{"ApiId": "other-api", "Definition": "type Query { hello: String }"},
	})
}

func newHandler(t *testing.T) (*GraphQLSchema, *fakeAPI) {
	t.Helper()
	api := newFakeAPI(awsfake.NewCloudControl(), apiID)
	return NewGraphQLSchema(api, api), api
}

// wait reads the progress of op until it finishes.
func wait(t *testing.T, h *GraphQLSchema, op native.Operation) native.Progress {
	t.Helper()
	for range 5 {
		progress, err := h.Progress(t.Context(), region, apiID, op)
		require.NoError(t, err)
		if progress.Done {
			return progress
		}
		require.Equal(t, "PROCESSING", progress.State)
	}
	require.Fail(t, "the operation did not finish")
	return native.Progress{}
}

func Test_DefinitionS3Location(t *testing.T) {
	h, api := newHandler(t)
	api.objects["schemas/app/schema.graphql"] = "type Query { fromS3: String }"

	identifier, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": apiID, "DefinitionS3Location": "s3://schemas/app/schema.graphql"}})
	require.NoError(t, err)
	require.Equal(t, apiID, identifier)
	require.Nil(t, wait(t, h, native.OperationCreate).Failure)
	require.Equal(t, "type Query { fromS3: String }", api.store.Resource(schemaType, apiID)["Definition"])
	require.Equal(t, []string{"GetObject " + region, "StartSchemaCreation " + region, "GetSchemaCreationStatus " + region, "GetSchemaCreationStatus " + region}, api.calls)

	got, err := h.Read(t.Context(), region, apiID)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"ApiId": apiID, "Id": apiID}, got)
}

func Test_BadDefinitionS3Location(t *testing.T) {
	h, api := newHandler(t)
	failure := &native.Failure{}
	for _, location := range []string{"", "https://schemas/a.graphql", "s3://schemas", "s3:///key", "s3://schemas/"} {
		properties := map[string]any{"ApiId": apiID}
		if location != "" {
			properties["DefinitionS3Location"] = location
		}
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
		require.ErrorAs(t, err, &failure, location)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	}

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": apiID, "DefinitionS3Location": "s3://schemas/missing.graphql"}})
	require.ErrorContains(t, err, "DefinitionS3Location")
	require.ErrorContains(t, err, "NoSuchKey")

	h.objects = brokenBody{}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": apiID, "DefinitionS3Location": "s3://schemas/broken.graphql"}})
	require.ErrorContains(t, err, "DefinitionS3Location")
	require.Empty(t, api.store.List(schemaType), "nothing reached AppSync")
}

// Test_BothDefinitions checks that a template with both Definition and DefinitionS3Location, which AppSync takes as
// alternatives, is rejected on create and update before any call.
func Test_BothDefinitions(t *testing.T) {
	h, api := newHandler(t)
	both := map[string]any{"ApiId": apiID, "Definition": "type Query { a: String }", "DefinitionS3Location": "s3://schemas/b.graphql"}
	requireRejected := func(err error) {
		t.Helper()
		failure := &native.Failure{}
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.Contains(t, failure.Message, "Definition or DefinitionS3Location, not both")
	}

	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: both})
	requireRejected(err)
	require.Empty(t, api.calls, "nothing reached AWS")

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": apiID, "Definition": "type Query { a: String }"}})
	require.NoError(t, err)
	wait(t, h, native.OperationCreate)
	api.calls = nil
	current := map[string]any{"ApiId": apiID, "Id": apiID}
	err = h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: apiID, Current: current, Desired: both, Patch: patchOf(t, `[{"op":"add","path":"/DefinitionS3Location","value":"s3://schemas/b.graphql"}]`)})
	requireRejected(err)
	require.Empty(t, api.calls, "nothing reached AWS")
}

func Test_FailedSchema(t *testing.T) {
	h, _ := newHandler(t)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": apiID, "Definition": "invalid {"}})
	require.NoError(t, err)
	progress := wait(t, h, native.OperationCreate)
	require.NotNil(t, progress.Failure)
	require.True(t, progress.Failure.ClientError)
	require.Equal(t, native.OutcomeRejected, progress.Failure.Outcome, "a failed schema changes nothing in AWS")
	require.Equal(t, "AppSync failed to create the schema: Syntax Error", progress.Failure.Message)
}

func Test_Update(t *testing.T) {
	h, api := newHandler(t)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": apiID, "Definition": "type Query { a: String }"}})
	require.NoError(t, err)
	wait(t, h, native.OperationCreate)
	current := map[string]any{"ApiId": apiID, "Id": apiID}
	api.calls = nil

	// A patch that changes no definition, and a template that no longer sets one, send nothing.
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: apiID, Current: current, Desired: current}))
	removed := native.UpdateRequest{Region: region, Identifier: apiID, Current: current, Desired: current, Patch: patchOf(t, `[{"op":"remove","path":"/Definition"}]`)}
	require.NoError(t, h.Update(t.Context(), removed))
	require.Empty(t, api.calls)

	api.objects["schemas/b.graphql"] = "type Query { b: String }"
	desired := map[string]any{"ApiId": apiID, "DefinitionS3Location": "s3://schemas/b.graphql"}
	moved := native.UpdateRequest{Region: region, Identifier: apiID, Current: current, Desired: desired, Patch: patchOf(t, `[{"op":"add","path":"/DefinitionS3Location","value":"s3://schemas/b.graphql"}]`)}
	require.NoError(t, h.Update(t.Context(), moved))
	require.Nil(t, wait(t, h, native.OperationUpdate).Failure)
	require.Equal(t, "type Query { b: String }", api.store.Resource(schemaType, apiID)["Definition"])
}

func Test_NotFound(t *testing.T) {
	h, api := newHandler(t)
	_, err := h.Read(t.Context(), region, apiID)
	require.ErrorIs(t, err, native.ErrNotFound)
	_, err = h.Progress(t.Context(), region, apiID, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"ApiId": "unknown", "Definition": "type Query { a: String }"}})
	require.Error(t, err, "AppSync rejects a schema for an API it doesn't have")

	boom := errors.New("boom")
	api.errs["GetGraphqlApi"] = boom
	_, err = h.Read(t.Context(), region, apiID)
	require.ErrorIs(t, err, boom)
}

func Test_NoDeleteOrList(t *testing.T) {
	h, api := newHandler(t)
	require.NoError(t, h.Delete(t.Context(), region, apiID))
	list, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, list)
	progress, err := h.Progress(t.Context(), region, apiID, native.OperationDelete)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.Empty(t, api.calls)
}

func Test_Progress(t *testing.T) {
	for state, done := range map[string]bool{"PROCESSING": false, "SUCCESS": true, "ACTIVE": true, "NOT_APPLICABLE": true, "FAILED": true} {
		progress := schemaStates.Progress(state)
		require.Equal(t, done, progress.Done, state)
		require.Equal(t, state == "FAILED", progress.Failure != nil, state)
	}
	require.True(t, strings.EqualFold(schemaStates.Progress("PROCESSING").Waiting, "SUCCESS"))
}

func patchOf(t *testing.T, raw string) jsondiff.Patch {
	t.Helper()
	patch := jsondiff.Patch{}
	require.NoError(t, json.Unmarshal([]byte(raw), &patch))
	return patch
}

// brokenBody is an S3 client whose object bodies fail to read.
type brokenBody struct{}

func (brokenBody) GetObject(ctx context.Context, in *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return &s3.GetObjectOutput{Body: io.NopCloser(iotest.ErrReader(errors.New("connection reset")))}, nil
}
