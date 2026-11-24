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
	"encoding/json"
	"errors"
	"maps"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms"
	"github.com/aws/aws-sdk-go-v2/service/lambdamicrovms/types"
	"github.com/stretchr/testify/require"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/" + file)
	require.NoError(t, err)
	return string(b)
}

// webApp returns a template for a MicroVM of the image, with changes.
func webApp(changes map[string]any) map[string]any {
	out := map[string]any{
		"ImageIdentifier":          image,
		"ExecutionRoleArn":         roleARN,
		"IdlePolicy":               map[string]any{"AutoResumeEnabled": true, "MaxIdleDurationSeconds": 900, "SuspendedDurationSeconds": 300},
		"IngressNetworkConnectors": []any{"arn:aws:lambda:us-west-2:aws:network-connector:aws-network-connector:ALL_INGRESS"},
		"EgressNetworkConnectors":  []any{},
		"MaximumDurationInSeconds": 3600,
		"Logging":                  map[string]any{"CloudWatch": map[string]any{"LogGroup": "/microvm/web-app"}},
		"RunHookPayload":           "session=4f1c",
	}
	maps.Copy(out, changes)
	return out
}

func newHandler() (*Microvm, *fakeAPI) {
	api := newFakeAPI(awsfake.NewCloudControl())
	return NewMicrovm(api), api
}

func Test_Suite_Microvm(t *testing.T) {
	// AWS reports the image by its ARN. Test_ImageTransform covers a template that names it by its name.
	properties := webApp(map[string]any{"ImageIdentifier": imageArn})
	nativetest.Run(t, nativetest.Suite{
		TypeName:    microvmType,
		Schema:      readFile(t, "aws-lambda-microvm.json"),
		Metadata:    readFile(t, "aws-lambda-microvm.metadata.json"),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return NewMicrovm(newFakeAPI(store))
		},
		IdentifierProperty: "MicrovmId",
		WriteOnly:          []string{"Logging", "RunHookPayload"},
		Properties:         properties,
		// Every property is create-only, so a deploy of the same template is the only update.
		Changed:          properties,
		CreateOnlyChange: webApp(map[string]any{"MaximumDurationInSeconds": 7200}),
		Invalid: []map[string]any{
			webApp(map[string]any{"ImageIdentifier": 5}),
			webApp(map[string]any{"Logging": map[string]any{"CloudWatch": map[string]any{"LogGroup": "/a"}, "Disabled": map[string]any{}}}),
			webApp(map[string]any{"IdlePolicy": map[string]any{"AutoResumeEnabled": true, "MaxIdleDurationSeconds": 90.5, "SuspendedDurationSeconds": 0}}),
			webApp(map[string]any{"IdlePolicy": map[string]any{}}),
		},
		API: microvmAPI,
	})
}

func Test_InRegion(t *testing.T) {
	opts := lambdamicrovms.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h, _ := newHandler()
	id, err := h.Identifier(webApp(nil))
	require.NoError(t, err)
	require.Empty(t, id, "AWS assigns the identifier")
	id, err = h.Identifier(map[string]any{"MicrovmId": firstID})
	require.NoError(t, err)
	require.Equal(t, firstID, id, "`existing` names the MicroVM")
}

// Test_ValueTypes checks that a value of the wrong type is rejected before any AWS call, instead of being sent as an
// empty or dropped value.
func Test_ValueTypes(t *testing.T) {
	h, api := newHandler()
	for name, c := range map[string]struct {
		properties map[string]any
		message    string
	}{
		"a string":     {webApp(map[string]any{"ImageIdentifier": 5}), "ImageIdentifier must be a string, not a number"},
		"a boolean":    {webApp(map[string]any{"IdlePolicy": map[string]any{"AutoResumeEnabled": "yes"}}), "IdlePolicy.AutoResumeEnabled must be a boolean, not a string"},
		"a list":       {webApp(map[string]any{"EgressNetworkConnectors": "internet"}), "EgressNetworkConnectors must be a list of strings, not a string"},
		"an object":    {webApp(map[string]any{"Logging": "off"}), "Logging must be an object, not a string"},
		"an integer":   {webApp(map[string]any{"MaximumDurationInSeconds": 1.5}), "MaximumDurationInSeconds must be an integer, not a number 1.5"},
		"both logging": {webApp(map[string]any{"Logging": map[string]any{"CloudWatch": map[string]any{}, "Disabled": map[string]any{}}}), "exactly one of CloudWatch and Disabled"},
		"no logging":   {webApp(map[string]any{"Logging": map[string]any{}}), "exactly one of CloudWatch and Disabled"},
		"idle policy":  {webApp(map[string]any{"IdlePolicy": map[string]any{"AutoResumeEnabled": true, "MaxIdleDurationSeconds": 60}}), "IdlePolicy of an AWS::Lambda::Microvm must set"},
	} {
		t.Run(name, func(t *testing.T) {
			var failure *native.Failure
			_, err := h.Identifier(c.properties)
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			require.Contains(t, failure.Message, c.message)
			_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: c.properties, ClientToken: "token"})
			require.ErrorAs(t, err, &failure, "Create rejects it too")
			require.Zero(t, api.runs, "nothing is run")
		})
	}
	require.Equal(t, "not JSON", typeError(errors.New("not JSON")), "an error that isn't about a type is reported as it is")
}

func Test_CreateNeedsImage(t *testing.T) {
	h, api := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{"MaximumDurationInSeconds": 60}, ClientToken: "token"})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.True(t, failure.ClientError)
	require.Zero(t, api.runs)
}

func Test_CreateAndRead(t *testing.T) {
	h, api := newHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: "token-a"})
	require.NoError(t, err)
	stored := api.store.Resource(microvmType, id)
	require.Equal(t, map[string]any{"CloudWatch": map[string]any{"LogGroup": "/microvm/web-app"}}, stored["Logging"], "AWS got the logging")
	require.Equal(t, "session=4f1c", stored["RunHookPayload"])

	properties, err := h.Read(t.Context(), region, id)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"MicrovmId":                id,
		"ImageIdentifier":          imageArn,
		"ImageArn":                 imageArn,
		"ImageVersion":             latestVersion,
		"ExecutionRoleArn":         roleARN,
		"IdlePolicy":               map[string]any{"AutoResumeEnabled": true, "MaxIdleDurationSeconds": int32(900), "SuspendedDurationSeconds": int32(300)},
		"IngressNetworkConnectors": []any{"arn:aws:lambda:us-west-2:aws:network-connector:aws-network-connector:ALL_INGRESS"},
		"EgressNetworkConnectors":  []any{},
		"MaximumDurationInSeconds": int32(3600),
		"Endpoint":                 "https://" + id + ".microvm.lambda.us-west-2.on.aws",
		"State":                    "PENDING",
	}, properties, "AWS returns neither Logging nor RunHookPayload, and the empty list the template set")

	again, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: "token-a"})
	require.NoError(t, err)
	require.Equal(t, id, again, "a create sent again with the same token returns the first MicroVM")
	require.Equal(t, 1, api.runs)

	id, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(map[string]any{"Logging": map[string]any{"Disabled": map[string]any{}}}), ClientToken: "token-b"})
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Disabled": map[string]any{}}, api.store.Resource(microvmType, id)["Logging"])
}

func Test_CreateRejected(t *testing.T) {
	h, _ := newHandler()
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(map[string]any{"ImageIdentifier": "missing"}), ClientToken: "token"})
	require.ErrorContains(t, err, notFoundCode, "AWS's error is returned")
}

func Test_ReadGone(t *testing.T) {
	h, _ := newHandler()
	_, err := h.Read(t.Context(), region, "mvm-missing")
	require.ErrorIs(t, err, native.ErrNotFound)

	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: "token"})
	require.NoError(t, err)
	require.NoError(t, h.Delete(t.Context(), region, id))
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound, "a TERMINATING MicroVM is not found")
	_, err = h.Read(t.Context(), region, id)
	require.ErrorIs(t, err, native.ErrNotFound, "a TERMINATED MicroVM is not found")
	require.NoError(t, h.Delete(t.Context(), region, id), "AWS terminates a terminated MicroVM again without error")
	require.ErrorIs(t, h.Delete(t.Context(), region, "mvm-missing"), native.ErrNotFound)
}

func Test_ReadFails(t *testing.T) {
	h, api := newHandler()
	api.errs["GetMicrovm"] = fault("ThrottlingException")
	_, err := h.Read(t.Context(), region, firstID)
	require.ErrorContains(t, err, "ThrottlingException")
	require.NotErrorIs(t, err, native.ErrNotFound)
	_, err = h.Progress(t.Context(), region, firstID, native.OperationCreate)
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_ProgressCreate(t *testing.T) {
	h, api := newHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: "token"})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "PENDING", Waiting: "RUNNING"}, progress)
	progress, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Done: true, State: "RUNNING"}, progress)

	api.state[id] = types.MicrovmStateSuspended
	progress, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{Done: true, State: "SUSPENDED"}, progress, "a MicroVM AWS suspended for lack of traffic ran")

	api.state[id] = types.MicrovmState("STARTING")
	progress, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "STARTING", Waiting: "RUNNING"}, progress, "a state the handler doesn't know isn't done")

	_, err = h.Progress(t.Context(), region, "mvm-missing", native.OperationCreate)
	require.ErrorIs(t, err, native.ErrNotFound)
}

// Test_ProgressCreateTerminated checks that a MicroVM that terminates before it runs fails the create, as applied, so
// UCP saves it and the next deploy runs a new one.
func Test_ProgressCreateTerminated(t *testing.T) {
	h, api := newHandler()
	api.failNext = true
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: "token"})
	require.NoError(t, err)
	_, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
	require.Equal(t, &native.Failure{
		Message:     `AWS reports the Lambda MicroVM "` + id + `" as TERMINATED before it ran (ApplicationStartupFailure). Delete the resource, then deploy again to run a new MicroVM`,
		ClientError: true,
		Outcome:     native.OutcomeApplied,
	}, progress.Failure)

	api.state[id] = types.MicrovmStateTerminating
	delete(api.reason, id)
	progress, err = h.Progress(t.Context(), region, id, native.OperationCreate)
	require.NoError(t, err)
	require.Equal(t, `AWS reports the Lambda MicroVM "`+id+`" as TERMINATING before it ran. Delete the resource, then deploy again to run a new MicroVM`, progress.Failure.Message)
}

func Test_ProgressDelete(t *testing.T) {
	h, api := newHandler()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: "token"})
	require.NoError(t, err)
	delete(api.state, id)
	progress, err := h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "RUNNING", Waiting: "TERMINATED"}, progress, "a read before AWS shows the delete waits")

	require.NoError(t, h.Delete(t.Context(), region, id))
	progress, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "TERMINATING", Waiting: "TERMINATED"}, progress)
	_, err = h.Progress(t.Context(), region, id, native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound, "a TERMINATED MicroVM is deleted")
	_, err = h.Progress(t.Context(), region, "mvm-missing", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)
}

func Test_Update(t *testing.T) {
	h, _ := newHandler()
	require.NoError(t, h.Update(t.Context(), native.UpdateRequest{Region: region}))
	progress, err := h.Progress(t.Context(), region, "anything", native.OperationUpdate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_List(t *testing.T) {
	h, api := newHandler()
	ids := []string{}
	for _, token := range []string{"a", "b", "c"} {
		id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: webApp(nil), ClientToken: token})
		require.NoError(t, err)
		ids = append(ids, id)
	}
	require.NoError(t, h.Delete(t.Context(), region, ids[1]))

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	require.Equal(t, []native.Resource{
		{Identifier: ids[0], Properties: map[string]any{"MicrovmId": ids[0], "ImageIdentifier": imageArn, "ImageArn": imageArn, "ImageVersion": latestVersion, "State": "PENDING"}},
		{Identifier: ids[2], Properties: map[string]any{"MicrovmId": ids[2], "ImageIdentifier": imageArn, "ImageArn": imageArn, "ImageVersion": latestVersion, "State": "PENDING"}},
	}, resources, "the third MicroVM is on the second page, and the terminated one is left out")

	api.errs["ListMicrovms"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

// Test_EmptyConnectors checks that a template that sets an empty connector list redeploys with no change: Read
// reports AWS's empty list, so it matches, and a template that leaves the list out still matches too.
func Test_EmptyConnectors(t *testing.T) {
	schema := []byte(readFile(t, "aws-lambda-microvm.json"))
	for _, name := range []string{"IngressNetworkConnectors", "EgressNetworkConnectors"} {
		for _, template := range []map[string]any{webApp(map[string]any{name: []any{}}), awsapi.Without(webApp(nil), name)} {
			h, _ := newHandler()
			id, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: template, ClientToken: "token"})
			require.NoError(t, err)
			read, err := h.Read(t.Context(), region, id)
			require.NoError(t, err)
			current, err := json.Marshal(read)
			require.NoError(t, err)
			desired, err := json.Marshal(awsapi.Without(template, "Logging", "RunHookPayload"))
			require.NoError(t, err)
			patch, err := awsoperations.GeneratePatchWithOptions(current, desired, schema, awsoperations.PatchOptions{})
			require.NoError(t, err)
			require.Empty(t, patch, "%s %v", name, template[name])
		}
	}
}

// Test_ImageTransform checks that a template naming the image matches the ARN AWS reports, with or without a record of
// the last deploy, such as after a lost create whose values are unresolved, and that another image doesn't.
func Test_ImageTransform(t *testing.T) {
	schema := []byte(readFile(t, "aws-lambda-microvm.json"))
	current := []byte(`{"ImageIdentifier": "` + imageArn + `"}`)
	for template, want := range map[string]int{image: 0, imageArn: 0, "app": 1, "api-app": 1} {
		patch, err := awsoperations.GeneratePatchWithOptions(current, []byte(`{"ImageIdentifier": "`+template+`"}`), schema, awsoperations.PatchOptions{Unresolved: []string{"ImageIdentifier"}})
		require.NoError(t, err)
		require.Len(t, patch, want, template)
	}
}
