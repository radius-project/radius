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

package amplify

import (
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// staging returns a template for the environment staging of the Gen 1 app, with changes.
func staging(changes map[string]any) map[string]any {
	out := map[string]any{"AppId": gen1App, "EnvironmentName": "staging", "StackName": "amplify-app-staging"}
	maps.Copy(out, changes)
	return out
}

func Test_Suite_BackendEnvironment(t *testing.T) {
	properties := staging(nil)
	nativetest.Run(t, nativetest.Suite{
		TypeName:    backendEnvironmentType,
		Schema:      readFile(t, "aws-amplify-backendenvironment.json"),
		Metadata:    readFile(t, "aws-amplify-backendenvironment.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewBackendEnvironment(newFakeAPI(store))
		},
		Properties: properties,
		Identifier: gen1App + "|staging",
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: staging(map[string]any{"StackName": "amplify-app-other"}),
		Invalid: []map[string]any{
			staging(map[string]any{"EnvironmentName": "staging|blue"}),
			staging(map[string]any{"StackName": 5}),
			staging(map[string]any{"AppId": []any{gen1App}}),
		},
		IdentifierProperties: func(identifier string) map[string]any {
			e, err := parseID(identifier)
			require.NoError(t, err)
			return map[string]any{"AppId": e.app, "EnvironmentName": e.name}
		},
		API: backendEnvironmentAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := amplify.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.Identifier(staging(nil))
	require.NoError(t, err)
	require.Equal(t, gen1App+"|staging", id)

	id, err = h.Identifier(map[string]any{"AppId": gen1App})
	require.NoError(t, err)
	require.Empty(t, id, "an environment without a name has no identifier")

	for name, properties := range map[string]map[string]any{
		"an app ID with |":                      staging(map[string]any{"AppId": "d1|d2"}),
		"a name with |":                         staging(map[string]any{"EnvironmentName": "a|b"}),
		"an app ID that isn't a string":         staging(map[string]any{"AppId": 7}),
		"a name that isn't a string":            staging(map[string]any{"EnvironmentName": true}),
		"a stack name that isn't a string":      staging(map[string]any{"StackName": map[string]any{}}),
		"deployment artifacts that aren't text": staging(map[string]any{"DeploymentArtifacts": 1.5}),
	} {
		t.Run(name, func(t *testing.T) {
			var failure *native.Failure
			_, err := h.Identifier(properties)
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: properties})
			require.ErrorAs(t, err, &failure, "Create rejects it too")
		})
	}
}

func Test_CreateNeedsNames(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBackendEnvironment(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"AppId": gen1App}})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Zero(t, api.store.Count(backendEnvironmentType))
}

func Test_CreateOptional(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"AppId": gen1App, "EnvironmentName": "dev", "StackName": nil}})
	require.NoError(t, err)
	properties, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"AppId":                 gen1App,
		"EnvironmentName":       "dev",
		"BackendEnvironmentArn": "arn:aws:amplify:us-west-2:123456789012:apps/" + gen1App + "/backendenvironments/dev",
	}, properties, "properties the template leaves out aren't sent or read")

	id, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(map[string]any{"DeploymentArtifacts": "amplify-app-deployment"})})
	require.NoError(t, err)
	properties, err = h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, "amplify-app-staging", properties["StackName"])
	require.Equal(t, "amplify-app-deployment", properties["DeploymentArtifacts"])
}

func Test_CreateRejected(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(map[string]any{"AppId": gen2App})})
	require.ErrorContains(t, err, badRequestCode, "AWS's error is returned")
	var failure *native.Failure
	require.NotErrorAs(t, err, &failure, "AWS has no environment, so the create is rejected")
}

// Test_CreateAlreadyExists covers an SDK retry of a create whose response was lost: AWS rejects the retry, and has the
// environment the first attempt made.
func Test_CreateAlreadyExists(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(nil)})
	require.NoError(t, err)
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(nil)})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome, "an earlier attempt may have made the environment, so UCP keeps the record")
	require.Contains(t, failure.Message, badRequestCode)
}

// Test_CreateRejectedReadFails covers a rejected create whose check for an environment an earlier attempt made fails,
// so UCP can't tell whether AWS has one and keeps the record.
func Test_CreateRejectedReadFails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBackendEnvironment(api)
	api.errs["CreateBackendEnvironment"] = fault(badRequestCode)
	api.errs["GetBackendEnvironment"] = &smithy.GenericAPIError{Code: "InternalFailureException", Fault: smithy.FaultServer}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(nil)})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomeUnknown, failure.Outcome)
	require.False(t, failure.ClientError)
	require.Contains(t, failure.Message, "InternalFailureException")
}

func Test_CreateServerError(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBackendEnvironment(api)
	api.errs["CreateBackendEnvironment"] = &smithy.GenericAPIError{Code: "InternalFailureException", Fault: smithy.FaultServer}
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(nil)})
	require.ErrorContains(t, err, "InternalFailureException")
	var failure *native.Failure
	require.NotErrorAs(t, err, &failure, "UCP treats a server error as unknown on its own")
}

func Test_BadIdentifier(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	for _, identifier := range []string{"staging", "|staging", gen1App + "|", gen1App + "|a|b"} {
		var failure *native.Failure
		_, err := h.Read(t.Context(), region, identifier)
		require.ErrorAs(t, err, &failure, identifier)
		require.True(t, failure.ClientError)
		require.ErrorAs(t, h.Delete(t.Context(), region, identifier), &failure)
		_, err = h.Progress(t.Context(), region, identifier, native.OperationCreate)
		require.ErrorAs(t, err, &failure)
	}
}

func Test_DeleteAndProgress(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBackendEnvironment(api)
	identifier := gen1App + "|staging"
	require.ErrorIs(t, h.Delete(t.Context(), region, identifier), native.ErrNotFound)
	_, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: staging(nil)})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), region, identifier, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Done: true}, progress)
	progress, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Waiting: "deleted"}, progress, "a delete waits while AWS still has the environment")

	require.NoError(t, h.Delete(t.Context(), region, identifier))
	_, err = h.Progress(t.Context(), region, identifier, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_Update(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region}))
	progress, err := h.Progress(t.Context(), region, "anything", native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_ReadFails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewBackendEnvironment(api)
	api.errs["GetBackendEnvironment"] = fault("UnauthorizedException")
	_, err := h.Read(t.Context(), region, gen1App+"|staging")
	require.ErrorContains(t, err, "UnauthorizedException")
	require.NotErrorIs(t, err, native.ErrNotFound)

	api.errs = map[string]error{}
	_, err = h.Read(t.Context(), region, "d0000000000000|staging")
	require.ErrorIs(t, err, native.ErrNotFound, "an app that doesn't exist has no environment")
}

func Test_List(t *testing.T) {
	h := NewBackendEnvironment(newFakeAPI(awsfake.NewCloudControl()))
	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Empty(t, resources)
}
