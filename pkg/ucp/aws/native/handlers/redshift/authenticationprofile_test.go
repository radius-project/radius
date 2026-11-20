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

package redshift

import (
	"encoding/json"
	"errors"
	"github.com/aws/aws-sdk-go-v2/service/redshift"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const profileTestRegion = "us-west-2"

// profileSchema returns the hand-written schema of AWS::Redshift::AuthenticationProfile.
func profileSchema(t *testing.T) string {
	b, err := os.ReadFile("../../../../../aws/schemas/handwritten/aws-redshift-authenticationprofile.json")
	require.NoError(t, err)
	return string(b)
}

// profileProps returns the properties of a profile with the name and content.
func profileProps(name string, content string) map[string]any {
	properties := map[string]any{"AuthenticationProfileContent": content}
	if name != "" {
		properties["AuthenticationProfileName"] = name
	}
	return properties
}

func Test_Suite_AuthenticationProfile(t *testing.T) {
	var mu sync.Mutex
	nativetest.Run(t, nativetest.Suite{
		TypeName:    profileType,
		Schema:      profileSchema(t),
		HandWritten: true,
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			mu.Lock()
			defer mu.Unlock()
			api := newProfileFake(store)
			api.awsForm = false
			return NewAuthenticationProfile(api)
		},
		IdentifierProperty: "AuthenticationProfileName",
		Properties:         profileProps("odbc", `{"AllowDBUserOverride": 1, "App_ID": "example"}`),
		Identifier:         "odbc",
		Changed:            profileProps("odbc", `{"AllowDBUserOverride":"0","App_ID":"example"}`),
		CreateOnlyChange:   profileProps("jdbc", `{"AllowDBUserOverride": 1, "App_ID": "example"}`),
		OtherForm:          map[string]any{"AuthenticationProfileContent": `{"AllowDBUserOverride":"1","App_ID":"example"}`},
		Invalid: []map[string]any{
			profileProps("odbc_profile", `{"App_ID": "x"}`),
			profileProps(strings.Repeat("a", 64), `{"App_ID": "x"}`),
			profileProps("odbc", `not json`),
			profileProps("odbc", `["App_ID"]`),
			profileProps("odbc", `{"App_ID": {"Name": "x"}}`),
			profileProps("odbc", `{"App_ID": "x"} {}`),
			profileProps("odbc", `{"App_ID": "x"}}`),
			profileProps("odbc", `{"App_ID": "x"}]`),
			{"AuthenticationProfileName": "odbc", "AuthenticationProfileContent": map[string]any{"App_ID": "x"}},
			{"AuthenticationProfileName": 5, "AuthenticationProfileContent": `{"App_ID": "x"}`},
		},
		API: profileAPI,
	})
}

// profileUpdate reads the profile and sends an update to desired, with the patch UCP computes from the template and
// the last deploy, lastApplied.
func profileUpdate(t *testing.T, h *AuthenticationProfile, lastApplied map[string]any, desired map[string]any) error {
	current, err := h.Read(t.Context(), profileTestRegion, "odbc")
	require.NoError(t, err)
	c, err := json.Marshal(current)
	require.NoError(t, err)
	d, err := json.Marshal(desired)
	require.NoError(t, err)
	patch, err := awsoperations.GeneratePatchWithOptions(c, d, []byte(profileSchema(t)), awsoperations.PatchOptions{LastApplied: lastApplied})
	require.NoError(t, err)
	return h.Update(t.Context(), native.UpdateRequest{Region: profileTestRegion, Identifier: "odbc", Current: current, Desired: desired, Patch: patch})
}

// profileCreated creates the profile properties give.
func profileCreated(t *testing.T, h *AuthenticationProfile, properties map[string]any) {
	t.Helper()
	id, err := h.Create(t.Context(), native.CreateRequest{Region: profileTestRegion, Properties: properties})
	require.NoError(t, err)
	progress, err := h.Progress(t.Context(), profileTestRegion, id, native.OperationCreate)
	require.NoError(t, err)
	require.True(t, progress.Done)
}

func Test_AuthenticationProfile_Update(t *testing.T) {
	api := newProfileFake(awsfake.NewCloudControl())
	h := NewAuthenticationProfile(api)
	created := profileProps("odbc", `{"b": 1, "a": "x"}`)
	profileCreated(t, h, created)

	require.NoError(t, profileUpdate(t, h, created, profileProps("odbc", `{"b":"1","a":"x"}`)))
	require.Zero(t, api.calls["ModifyAuthenticationProfile"], "a template in AWS's form sends nothing")

	require.NoError(t, profileUpdate(t, h, created, profileProps("odbc", `{"b": 2}`)))
	require.Equal(t, 1, api.calls["ModifyAuthenticationProfile"])
	stored, err := h.Read(t.Context(), profileTestRegion, "odbc")
	require.NoError(t, err)
	require.Equal(t, `{"b":"2"}`, stored["AuthenticationProfileContent"], "the new content replaces the whole content")

	patch := jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AuthenticationProfileContent", Value: `{"b": [1]}`}}
	for _, desired := range []map[string]any{profileProps("odbc", `{"b": [1]}`), {"AuthenticationProfileName": "odbc"}} {
		err = h.Update(t.Context(), native.UpdateRequest{Region: profileTestRegion, Identifier: "odbc", Desired: desired, Patch: patch})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, "%v", desired)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	}
	require.Equal(t, 1, api.calls["ModifyAuthenticationProfile"])
}

func Test_profileEquivalent(t *testing.T) {
	for _, tt := range []struct {
		template any
		stored   any
		want     bool
	}{
		{`{"a": 1, "b": true, "c": null}`, `{"a":"1","b":"true"}`, true},
		{`{"a": 1.50}`, `{"a":"1.50"}`, true},
		{`{"a": 1.50}`, `{"a":"1.5"}`, false},
		{`{"b": "1", "a": "2"}`, `{"a":"2","b":"1"}`, true},
		{`{"a": "1"}`, `{"a":"1","b":"2"}`, false},
		{`{"a": "1"}`, `{"a":"2"}`, false},
		{`{"a": {"b": 1}}`, `{"a":{"b":1}}`, false},
		{`not json`, `{"a":"1"}`, false},
		{`{"a": "1"}}`, `{"a":"1"}`, false},
		{`{"a": "1"}]`, `{"a":"1"}`, false},
		{`{"a": "1"} `, `{"a":"1"}`, true},
		{`{"a": "1"}`, `not json`, false},
		{5, `{"a":"1"}`, false},
		{`{"a": "1"}`, 5, false},
	} {
		require.Equal(t, tt.want, profileEquivalent("AuthenticationProfileContent", tt.template, tt.stored), "%v %v", tt.template, tt.stored)
	}
	require.False(t, profileEquivalent("AuthenticationProfileName", "a", "a"))
}

func Test_AuthenticationProfile_Errors(t *testing.T) {
	api := newProfileFake(awsfake.NewCloudControl())
	h := NewAuthenticationProfile(api)

	_, err := h.Read(t.Context(), profileTestRegion, "missing")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Delete(t.Context(), profileTestRegion, "missing"), native.ErrNotFound)
	err = h.Update(t.Context(), native.UpdateRequest{Region: profileTestRegion, Identifier: "missing", Desired: profileProps("missing", `{"a":"1"}`),
		Patch: jsondiff.Patch{{Type: jsondiff.OperationReplace, Path: "/AuthenticationProfileContent", Value: `{"a":"1"}`}}})
	require.ErrorIs(t, err, native.ErrNotFound)

	for _, properties := range []map[string]any{
		{"AuthenticationProfileContent": `{"a":"1"}`},
		{"AuthenticationProfileName": "odbc"},
	} {
		_, err = h.Create(t.Context(), native.CreateRequest{Region: profileTestRegion, Properties: properties})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure, "%v", properties)
		require.True(t, failure.ClientError)
	}
	_, err = h.Create(t.Context(), native.CreateRequest{Region: profileTestRegion, Properties: profileProps("odbc_profile", `{"a":"1"}`)})
	require.ErrorAs(t, err, new(*native.Failure))
	require.Zero(t, api.calls["CreateAuthenticationProfile"], "an invalid value is never sent")
	_, err = h.Create(t.Context(), native.CreateRequest{Region: profileTestRegion, Properties: profileProps("odbc", `{"Password":"x"}`)})
	require.ErrorContains(t, err, profileInvalid, "AWS's error is returned as it is; the real SDK's is a 400, which awsapi.Rejected reports as rejected")
	require.Zero(t, api.store.Count(profileType))

	profileCreated(t, h, profileProps("odbc", `{"a":"1"}`))
	require.NoError(t, h.Delete(t.Context(), profileTestRegion, "odbc"))
	progress, err := h.Progress(t.Context(), profileTestRegion, "odbc", native.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, native.Progress{State: "listed", Waiting: "deleted"}, progress, "AWS may still list a deleted profile")
	_, err = h.Progress(t.Context(), profileTestRegion, "odbc", native.OperationDelete)
	require.ErrorIs(t, err, native.ErrNotFound)

	api.errs["DescribeAuthenticationProfiles"] = profileFault("Throttling", "slow down")
	_, err = h.Progress(t.Context(), profileTestRegion, "odbc", native.OperationCreate)
	require.ErrorContains(t, err, "Throttling")
	_, err = h.List(t.Context(), profileTestRegion)
	require.ErrorContains(t, err, "Throttling")
}

func Test_AuthenticationProfile_List(t *testing.T) {
	api := newProfileFake(awsfake.NewCloudControl())
	h := NewAuthenticationProfile(api)
	profileCreated(t, h, profileProps("odbc", `{"a":"1"}`))
	profileCreated(t, h, profileProps("jdbc", `{"a":"2"}`))
	resources, err := h.List(t.Context(), profileTestRegion)
	require.NoError(t, err)
	require.Equal(t, []native.Resource{
		{Identifier: "jdbc", Properties: map[string]any{"AuthenticationProfileName": "jdbc", "AuthenticationProfileContent": `{"a":"2"}`}},
		{Identifier: "odbc", Properties: map[string]any{"AuthenticationProfileName": "odbc", "AuthenticationProfileContent": `{"a":"1"}`}},
	}, resources)
}

// Test_AuthenticationProfile_LostResponse checks that a create AWS may have applied, whose response was lost, is
// never reported as rejected. The SDK would send it again and AWS would answer AuthenticationProfileAlreadyExistsFault,
// which would drop the record of a profile AWS made.
func Test_AuthenticationProfile_LostResponse(t *testing.T) {
	cfg, calls := firstResponseLost(t, profileExists)
	_, err := NewAuthenticationProfile(redshift.NewFromConfig(cfg)).Create(t.Context(), native.CreateRequest{Region: profileTestRegion, Properties: profileProps("odbc", `{"a":"1"}`)})
	require.Error(t, err)
	var failure *native.Failure
	require.False(t, errors.As(err, &failure), "a lost response is an unknown outcome, not %v", failure)
	require.Equal(t, 1, *calls, "the SDK doesn't send a create again")
}

// Test_AuthenticationProfile_LostDelete checks that a delete the SDK sends again after a lost response, which AWS
// answers with AuthenticationProfileNotFoundFault, finishes the delete.
func Test_AuthenticationProfile_LostDelete(t *testing.T) {
	cfg, calls := firstResponseLost(t, profileNotFound)
	err := NewAuthenticationProfile(redshift.NewFromConfig(cfg)).Delete(t.Context(), profileTestRegion, "odbc")
	require.ErrorIs(t, err, native.ErrNotFound)
	require.Equal(t, 2, *calls)
}
