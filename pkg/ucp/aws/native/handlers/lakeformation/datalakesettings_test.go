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

package lakeformation

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation"
	"github.com/aws/aws-sdk-go-v2/service/lakeformation/types"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

// patchOf returns the patch from current to desired for the properties desired sets and the removed ones, as UCP
// computes it.
func patchOf(t *testing.T, current map[string]any, desired map[string]any, removed ...string) jsondiff.Patch {
	relevant := map[string]any{}
	for _, names := range [][]string{keys(desired), removed} {
		for _, k := range names {
			if v, ok := current[k]; ok {
				relevant[k] = v
			}
		}
	}
	a, err := json.Marshal(relevant)
	require.NoError(t, err)
	b, err := json.Marshal(desired)
	require.NoError(t, err)
	patch, err := jsondiff.CompareJSON(a, b)
	require.NoError(t, err)
	return patch
}

func keys(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

func newSettings(t *testing.T, stored map[string]any) (*DataLakeSettings, *fakeAPI) {
	store := awsfake.NewCloudControl()
	if stored != nil {
		store.Put(settingsType, settingsIdentifier, stored)
	}
	api := newFakeAPI(store)
	return NewDataLakeSettings(api), api
}

func awsSettings(t *testing.T, api *fakeAPI) *types.DataLakeSettings {
	out, err := api.GetDataLakeSettings(t.Context(), &lakeformation.GetDataLakeSettingsInput{})
	require.NoError(t, err)
	return out.DataLakeSettings
}

func update(t *testing.T, h native.Handler, current map[string]any, desired map[string]any, removed ...string) error {
	return h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: settingsIdentifier, Current: current, Desired: desired, Patch: patchOf(t, current, desired, removed...),
	})
}

// Test_DataLakeSettings_Append deploys admins with MutationType APPEND to settings that have an admin added outside
// Radius. Every deploy keeps that admin and doesn't see it as a change, until the template replaces the admins.
func Test_DataLakeSettings_Append(t *testing.T) {
	ctx := t.Context()
	h, api := newSettings(t, map[string]any{"Admins": admins(outside)})

	first := map[string]any{"MutationType": "APPEND", "Admins": admins(adminA)}
	id, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: first})
	require.NoError(t, err)
	require.Equal(t, settingsIdentifier, id)
	all, err := h.Read(ctx, region, settingsIdentifier)
	require.NoError(t, err)
	require.Equal(t, admins(outside, adminA), all["Admins"])
	require.Equal(t, settingsIdentifier, all["Id"])

	// Last-applied properties never hold the write-only MutationType.
	applied := map[string]any{"Admins": admins(adminA)}
	second := map[string]any{"MutationType": "append", "Admins": admins(adminB)}
	current, err := h.ReadScoped(ctx, region, settingsIdentifier, []map[string]any{applied, second})
	require.NoError(t, err)
	require.Equal(t, admins(adminA), current["Admins"], "an admin added outside Radius is not a change")
	require.NoError(t, update(t, h, current, second))
	require.Equal(t, []string{outside, adminB}, sdkPrincipalIDs(awsSettings(t, api).DataLakeAdmins))

	applied = map[string]any{"Admins": admins(adminB)}
	current, err = h.ReadScoped(ctx, region, settingsIdentifier, []map[string]any{applied, second})
	require.NoError(t, err)
	require.Equal(t, admins(adminB), current["Admins"], "a redeploy sees no change")

	// Removing Admins removes only the managed admins.
	third := map[string]any{"MutationType": "APPEND"}
	current, err = h.ReadScoped(ctx, region, settingsIdentifier, []map[string]any{applied, third})
	require.NoError(t, err)
	require.NoError(t, update(t, h, current, third, "Admins"))
	require.Equal(t, []string{outside}, sdkPrincipalIDs(awsSettings(t, api).DataLakeAdmins))

	// No managed admin is left, and the settings still exist.
	current, err = h.ReadScoped(ctx, region, settingsIdentifier, []map[string]any{third})
	require.NoError(t, err)
	require.NotContains(t, current, "Admins")

	// Without APPEND, the template's admins replace every admin.
	replace := map[string]any{"Admins": admins(adminB)}
	current, err = h.ReadScoped(ctx, region, settingsIdentifier, []map[string]any{{}, replace})
	require.NoError(t, err)
	require.Equal(t, admins(outside), current["Admins"])
	require.NoError(t, update(t, h, current, replace))
	require.Equal(t, []string{adminB}, sdkPrincipalIDs(awsSettings(t, api).DataLakeAdmins))
}

// Test_DataLakeSettings_AppendRemovalFails fails an APPEND update that removes a managed admin. The failure is
// rejected, so the record keeps the admin and the next deploy removes it. Other failed updates keep their error.
func Test_DataLakeSettings_AppendRemovalFails(t *testing.T) {
	appendAdmins := func(ids ...string) map[string]any {
		return map[string]any{"MutationType": "APPEND", "Admins": admins(ids...)}
	}
	for _, tt := range []struct {
		name     string
		call     string
		err      error
		current  map[string]any
		desired  map[string]any
		rejected bool
		client   bool
	}{
		{"read fails", "GetDataLakeSettings", nativefake.ServerError("InternalServiceException", "failed"), appendAdmins(adminA, adminB), appendAdmins(adminA), true, false},
		{"put fails", "PutDataLakeSettings", nativefake.ServerError("InternalServiceException", "failed"), appendAdmins(adminA, adminB), appendAdmins(adminA), true, false},
		{"put is denied", "PutDataLakeSettings", nativefake.ClientError("AccessDeniedException", "denied"), appendAdmins(adminA, adminB), appendAdmins(adminA), true, true},
		{"an update that only adds admins fails", "PutDataLakeSettings", nativefake.ServerError("InternalServiceException", "failed"), appendAdmins(adminA), appendAdmins(adminA, adminB), false, false},
		{"an update without APPEND fails", "PutDataLakeSettings", nativefake.ServerError("InternalServiceException", "failed"), map[string]any{"Admins": admins(adminA, adminB)}, map[string]any{"Admins": admins(adminA)}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, api := newSettings(t, map[string]any{"Admins": admins(outside, adminA, adminB)})
			api.errs[tt.call] = tt.err
			err := update(t, h, tt.current, tt.desired)
			var failure *native.Failure
			if !tt.rejected {
				require.ErrorIs(t, err, tt.err)
				require.False(t, errors.As(err, &failure))
				return
			}
			require.ErrorAs(t, err, &failure)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
			require.Equal(t, tt.client, failure.ClientError)
			delete(api.errs, tt.call)
			require.Equal(t, []string{outside, adminA, adminB}, sdkPrincipalIDs(awsSettings(t, api).DataLakeAdmins))
		})
	}
}

// Test_DataLakeSettings_Removed removes every setting from the template. Each goes back to AWS's default, except
// Parameters, which keeps AWS's value.
func Test_DataLakeSettings_Removed(t *testing.T) {
	principal := map[string]any{"DataLakePrincipalIdentifier": analyst}
	defaults := []any{map[string]any{"Principal": principal, "Permissions": list("SELECT")}}
	h, api := newSettings(t, map[string]any{
		"Admins":                           admins(adminA),
		"ReadOnlyAdmins":                   admins(reader),
		"AllowExternalDataFiltering":       true,
		"AllowFullTableExternalDataAccess": true,
		"ExternalDataFilteringAllowList":   admins(analyst),
		"AuthorizedSessionTagValueList":    list("engine"),
		"CreateDatabaseDefaultPermissions": defaults,
		"CreateTableDefaultPermissions":    defaults,
		"TrustedResourceOwners":            list("111122223333"),
		"Parameters":                       map[string]any{"CROSS_ACCOUNT_VERSION": "4"},
	})
	current, err := h.Read(t.Context(), region, settingsIdentifier)
	require.NoError(t, err)
	removed := keys(current)
	require.NoError(t, update(t, h, current, map[string]any{}, removed...))

	got := awsSettings(t, api)
	iamOnly := []types.PrincipalPermissions{{
		Principal:   &types.DataLakePrincipal{DataLakePrincipalIdentifier: aws.String(iamAllowedPrincipals)},
		Permissions: []types.Permission{types.PermissionAll},
	}}
	require.Equal(t, &types.DataLakeSettings{
		AllowExternalDataFiltering:       aws.Bool(false),
		AllowFullTableExternalDataAccess: aws.Bool(false),
		CreateDatabaseDefaultPermissions: iamOnly,
		CreateTableDefaultPermissions:    iamOnly,
		Parameters:                       map[string]string{"CROSS_ACCOUNT_VERSION": "4"},
	}, got)
}

// Test_DataLakeSettings_Parameters changes, adds, and removes parameters one at a time, and keeps the ones AWS sets.
func Test_DataLakeSettings_Parameters(t *testing.T) {
	h, api := newSettings(t, map[string]any{"Parameters": map[string]any{"CROSS_ACCOUNT_VERSION": "3", "SET_CONTEXT": "TRUE", "a/b~c": "1"}})
	err := h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: settingsIdentifier,
		Desired: map[string]any{"Parameters": map[string]any{"CROSS_ACCOUNT_VERSION": "4", "NEW": "x"}},
		Patch: jsondiff.Patch{
			{Type: jsondiff.OperationReplace, Path: "/Parameters/CROSS_ACCOUNT_VERSION", Value: "4"},
			{Type: jsondiff.OperationRemove, Path: "/Parameters/a~1b~0c"},
			{Type: jsondiff.OperationAdd, Path: "/Parameters/NEW", Value: "x"},
			{Type: jsondiff.OperationAdd, Path: "/ParametersOther", Value: "ignored"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"CROSS_ACCOUNT_VERSION": "4", "SET_CONTEXT": "TRUE", "NEW": "x"}, awsSettings(t, api).Parameters)

	h, api = newSettings(t, map[string]any{"TrustedResourceOwners": list("111122223333")})
	err = h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: settingsIdentifier,
		Desired: map[string]any{"Parameters": map[string]any{"CROSS_ACCOUNT_VERSION": "4"}},
		Patch:   jsondiff.Patch{{Type: jsondiff.OperationAdd, Path: "/Parameters", Value: map[string]any{"CROSS_ACCOUNT_VERSION": "4"}}},
	})
	require.NoError(t, err)
	got := awsSettings(t, api)
	require.Equal(t, map[string]string{"CROSS_ACCOUNT_VERSION": "4"}, got.Parameters)
	require.Equal(t, []string{"111122223333"}, got.TrustedResourceOwners, "a setting the patch doesn't change stays")

	h, api = newSettings(t, map[string]any{"TrustedResourceOwners": list("111122223333")})
	err = h.Update(t.Context(), native.UpdateRequest{
		Region: region, Identifier: settingsIdentifier,
		Desired: map[string]any{"Parameters": map[string]any{"K": "V"}},
		Patch:   jsondiff.Patch{{Type: jsondiff.OperationAdd, Path: "/Parameters/K", Value: "V"}},
	})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"K": "V"}, awsSettings(t, api).Parameters)
}

// Test_DataLakeSettings_Create sets the template's settings over the ones AWS has.
func Test_DataLakeSettings_Create(t *testing.T) {
	h, api := newSettings(t, map[string]any{
		"Admins":                admins(outside),
		"TrustedResourceOwners": list("111122223333"),
		"Parameters":            map[string]any{"SET_CONTEXT": "TRUE"},
	})
	_, err := h.Create(t.Context(), native.CreateRequest{Region: region, Properties: map[string]any{
		"Admins":                     admins(adminA),
		"AllowExternalDataFiltering": true,
		"Parameters":                 map[string]any{"CROSS_ACCOUNT_VERSION": "4"},
	}})
	require.NoError(t, err)
	got := awsSettings(t, api)
	require.Equal(t, []string{adminA}, sdkPrincipalIDs(got.DataLakeAdmins), "without APPEND, the template's admins replace them")
	require.Equal(t, []string{"111122223333"}, got.TrustedResourceOwners)
	require.True(t, aws.ToBool(got.AllowExternalDataFiltering))
	require.Equal(t, map[string]string{"SET_CONTEXT": "TRUE", "CROSS_ACCOUNT_VERSION": "4"}, got.Parameters)
}

func Test_DataLakeSettings_Errors(t *testing.T) {
	ctx := t.Context()
	h, api := newSettings(t, nil)
	_, err := h.Read(ctx, region, settingsIdentifier)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, update(t, h, map[string]any{}, map[string]any{"TrustedResourceOwners": list("1")}), native.ErrNotFound)

	_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"Admins": "not a list"}})
	require.ErrorContains(t, err, "failed to read the properties")

	api.errs["PutDataLakeSettings"] = nativefake.ClientError("AccessDeniedException", "denied")
	_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"Admins": admins(adminA)}})
	require.ErrorContains(t, err, "AccessDeniedException")

	failure := nativefake.ServerError("InternalServiceException", "failed")
	api.errs["GetDataLakeSettings"] = failure
	_, err = h.Read(ctx, region, settingsIdentifier)
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, native.ErrNotFound)
	_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{}})
	require.ErrorIs(t, err, failure)

	h, _ = newSettings(t, map[string]any{})
	require.ErrorContains(t, update(t, h, map[string]any{}, map[string]any{"Admins": "not a list"}), "failed to read the properties")

	h, api = newSettings(t, nil)
	api.empty = true
	properties, err := h.Read(ctx, region, settingsIdentifier)
	require.NoError(t, err)
	require.Equal(t, map[string]any{"Id": settingsIdentifier}, properties)
}

// Test_Unsupported checks the calls UCP never makes for a type, and that every operation finishes at once.
func Test_Unsupported(t *testing.T) {
	ctx := t.Context()
	settings := NewDataLakeSettings(nil)
	require.NoError(t, settings.Delete(ctx, region, settingsIdentifier))
	require.NoError(t, settings.DeleteScoped(ctx, region, settingsIdentifier, nil))
	for _, h := range []native.Handler{settings, NewPermissions(nil), NewResource(nil)} {
		if h.Options().List == native.ListNone {
			resources, err := h.List(ctx, region)
			require.NoError(t, err)
			require.Empty(t, resources)
		}
		for _, op := range []native.Operation{native.OperationCreate, native.OperationUpdate, native.OperationDelete} {
			progress, err := h.Progress(ctx, region, "id", op)
			require.NoError(t, err)
			require.True(t, progress.Done)
		}
	}
}

func Test_InRegion(t *testing.T) {
	o := lakeformation.Options{}
	inRegion("eu-west-2")(&o)
	require.Equal(t, "eu-west-2", o.Region)
}
