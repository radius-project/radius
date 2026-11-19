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
	"errors"
	"maps"
	"os"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sagemaker"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const region = "us-west-2"

func schema(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

// fakes maps each handler the suite makes to its fake API.
type fakes struct {
	mu  sync.Mutex
	api map[native.Handler]*fakeAPI
}

func (f *fakes) add(h native.Handler, api *fakeAPI) native.Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api == nil {
		f.api = map[native.Handler]*fakeAPI{}
	}
	f.api[h] = api
	return h
}

func (f *fakes) of(h native.Handler) *fakeAPI {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.api[h]
}

func with(properties map[string]any, changes map[string]any) map[string]any {
	out := maps.Clone(properties)
	for k, v := range changes {
		if v == nil {
			delete(out, k)
		} else {
			out[k] = v
		}
	}
	return out
}

func tags(pairs ...string) []any {
	out := []any{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"Key": pairs[i], "Value": pairs[i+1]})
	}
	return out
}

var trial = map[string]any{
	"TrialComponentName": "app-trial",
	"DisplayName":        "App-trial",
	"StartTime":          "2026-01-02T03:04:05Z",
	"Status":             map[string]any{"PrimaryStatus": "InProgress"},
	"MetadataProperties": map[string]any{"ProjectId": "p-1"},
	"Tags":               tags("team", "a"),
}

func Test_Suite_TrialComponent(t *testing.T) {
	var fs fakes
	nativetest.Run(t, nativetest.Suite{
		TypeName: trialComponentType,
		Schema:   schema(t, "aws-sagemaker-experimenttrialcomponent.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			api := newFakeAPI(store)
			return fs.add(NewTrialComponent(api), api)
		},
		IdentifierProperty: "TrialComponentName",
		Properties:         trial,
		Identifier:         "app-trial",
		Changed:            with(trial, map[string]any{"DisplayName": "Renamed", "EndTime": "2026-01-03T00:00:00Z", "Status": map[string]any{"PrimaryStatus": "Completed"}}),
		Removable:          "Tags",
		CreateOnlyChange:   with(trial, map[string]any{"MetadataProperties": map[string]any{"ProjectId": "p-2"}}),
		OtherForm:          map[string]any{"StartTime": "2026-01-01T19:04:05.000-08:00"},
		MultiCall:          with(trial, map[string]any{"DisplayName": "Three-calls", "Tags": tags("env", "dev")}),
		Calls:              3,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &fs.of(h).failer
		}),
		API: trialComponentAPI,
	})
}

func Test_Equivalent(t *testing.T) {
	tests := []struct {
		path     string
		template any
		aws      any
		want     bool
	}{
		{"StartTime", "2026-01-02T03:04:05Z", "2026-01-02T03:04:05.000Z", true},
		{"EndTime", "2026-01-02T03:04:05+01:00", "2026-01-02T02:04:05Z", true},
		{"StartTime", "2026-01-02T03:04:05Z", "2026-01-02T03:04:06Z", false},
		{"StartTime", "not a time", "2026-01-02T03:04:05Z", false},
		{"StartTime", 5, "2026-01-02T03:04:05Z", false},
		{"DisplayName", "2026-01-02T03:04:05Z", "2026-01-02T03:04:05.000Z", false},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, equivalent(tt.path, tt.template, tt.aws), "%s %v %v", tt.path, tt.template, tt.aws)
	}
}

func Test_InRegion(t *testing.T) {
	opts := sagemaker.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func Test_Identifier(t *testing.T) {
	h := NewTrialComponent(newFakeAPI(awsfake.NewCloudControl()))
	id, err := h.Identifier(trial)
	require.NoError(t, err)
	require.Equal(t, "app-trial", id)
	id, err = h.Identifier(map[string]any{})
	require.NoError(t, err)
	require.Empty(t, id)
}

func Test_Create_InvalidTime(t *testing.T) {
	h := NewTrialComponent(newFakeAPI(awsfake.NewCloudControl()))
	for _, name := range timeProperties {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(trial, map[string]any{name: "Jan 2 2026"})})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.Contains(t, failure.Message, name)
		require.NotContains(t, failure.Message, "Jan 2 2026", "the message names the property, not its value")
	}
}

func Test_Create_Fails(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTrialComponent(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(trial, map[string]any{"Status": "InProgress"})})
	require.Error(t, err, "Status must be an object")

	api.errs["CreateTrialComponent"] = fault("ValidationException")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: trial})
	require.ErrorContains(t, err, "ValidationException")
}

func Test_Read_Errors(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTrialComponent(api)
	_, err := h.Read(t.Context(), region, "missing")
	require.ErrorIs(t, err, native.ErrNotFound)

	_, err = h.Create(t.Context(), native.CreateRequest{Region: region, Properties: trial})
	require.NoError(t, err)
	api.errs["ListTags"] = fault("ThrottlingException")
	_, err = h.Read(t.Context(), region, "app-trial")
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_Update(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTrialComponent(api)
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: trial})
	require.NoError(t, err)
	current, err := h.Read(t.Context(), region, "app-trial")
	require.NoError(t, err)
	update := func(desired map[string]any, changed ...string) error {
		patch := patchOf(changed...)
		return h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "app-trial", Current: current, Desired: desired, Patch: patch})
	}

	t.Run("a removed value keeps AWS's value", func(t *testing.T) {
		require.NoError(t, update(with(trial, map[string]any{"DisplayName": nil, "StartTime": nil, "Status": nil}), "DisplayName", "StartTime", "Status"))
		props, err := h.Read(t.Context(), region, "app-trial")
		require.NoError(t, err)
		require.Equal(t, "App-trial", props["DisplayName"])
		require.Equal(t, "2026-01-02T03:04:05Z", props["StartTime"])
	})

	t.Run("a new start time is set", func(t *testing.T) {
		require.NoError(t, update(with(trial, map[string]any{"StartTime": "2026-02-01T00:00:00Z"}), "StartTime"))
		props, err := h.Read(t.Context(), region, "app-trial")
		require.NoError(t, err)
		require.Equal(t, "2026-02-01T00:00:00Z", props["StartTime"])
	})

	t.Run("an invalid time is rejected", func(t *testing.T) {
		var failure *native.Failure
		require.ErrorAs(t, update(with(trial, map[string]any{"EndTime": "tomorrow"}), "EndTime"), &failure)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	})

	t.Run("an invalid status is rejected", func(t *testing.T) {
		require.Error(t, update(with(trial, map[string]any{"Status": map[string]any{"PrimaryStatus": 5}}), "Status"))
	})

	t.Run("a missing trial component is not found", func(t *testing.T) {
		err := h.Update(t.Context(), native.UpdateRequest{Region: region, Identifier: "missing", Desired: trial})
		require.ErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_Delete_NotFound(t *testing.T) {
	h := NewTrialComponent(newFakeAPI(awsfake.NewCloudControl()))
	require.ErrorIs(t, h.Delete(t.Context(), region, "missing"), native.ErrNotFound)
}

func Test_List(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTrialComponent(api)
	for _, name := range []string{"one", "two", "three"} {
		_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: with(trial, map[string]any{"TrialComponentName": name})})
		require.NoError(t, err)
	}
	require.NoError(t, h.Delete(t.Context(), region, "two"))

	resources, err := h.List(t.Context(), region)
	require.NoError(t, err)
	ids := []string{}
	for _, r := range resources {
		ids = append(ids, r.Identifier)
		require.Equal(t, r.Identifier, r.Properties["Id"])
	}
	require.Equal(t, []string{"one", "three"}, ids, "a trial component gone since the list is left out")

	api.errs["ListTags"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")

	api.errs["ListTrialComponents"] = fault("ThrottlingException")
	_, err = h.List(t.Context(), region)
	require.ErrorContains(t, err, "ThrottlingException")
}

func Test_Progress(t *testing.T) {
	api := newFakeAPI(awsfake.NewCloudControl())
	h := NewTrialComponent(api)
	progress, err := h.Progress(t.Context(), region, "missing", native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done, "a create takes effect at once")

	_, err = h.Progress(t.Context(), region, "missing", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	api.errs["DescribeTrialComponent"] = errors.New("boom")
	_, err = h.Progress(t.Context(), region, "app-trial", native.OperationDelete)
	require.EqualError(t, err, "boom")
}

// patchOf returns a patch that replaces each of the properties.
func patchOf(properties ...string) jsondiff.Patch {
	patch := jsondiff.Patch{}
	for _, p := range properties {
		patch = append(patch, jsondiff.Operation{Type: "replace", Path: "/" + p})
	}
	return patch
}
