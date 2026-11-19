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

package iam

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/stretchr/testify/require"
	"github.com/wI2L/jsondiff"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/awsapi"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativetest"
)

const (
	region   = "us-west-2"
	token    = "11111111-2222-3333-4444-555555555555"
	outsider = "outsider"
)

func readFile(t *testing.T, file string) string {
	b, err := os.ReadFile("../../../../../aws/schemas/native/" + file)
	require.NoError(t, err)
	return string(b)
}

func group(name string, users ...string) map[string]any {
	list := []any{}
	for _, u := range users {
		list = append(list, u)
	}
	return map[string]any{"GroupName": name, "Users": list}
}

func Test_Suite_UserToGroupAddition(t *testing.T) {
	nativetest.Run(t, nativetest.Suite{
		TypeName: userToGroupAdditionType,
		Schema:   readFile(t, "aws-iam-usertogroupaddition.json"),
		Metadata: readFile(t, "aws-iam-usertogroupaddition.metadata.json"),
		NewHandler: func(store *awsfake.CloudControl) native.Handler {
			return newTracked(store)
		},
		IdentifierProperty: "Id",
		Properties:         group("admins", "ana", "bo"),
		Changed:            group("admins", "bo", "cy"),
		// IAM user names ignore case, so BO is bo.
		SharedMember:     &nativetest.SharedMember{Overlapping: group("admins", "BO", "cy"), Disjoint: group("admins", "cy")},
		CreateOnlyChange: group("readers", "ana", "bo"),
		OtherForm:        map[string]any{"Users": []any{"bo", "ana"}},
		MultiCall:        group("admins", "bo", "cy", "di"),
		Calls:            3,
		RejectedCalls:    1,
		FailCall: nativetest.FailCall(3, func(h native.Handler) *nativetest.Failer {
			return &h.(*tracked).api.failer
		}),
		API: userToGroupAdditionAPI,
		Outside: func(store *awsfake.CloudControl, properties map[string]any) func() bool {
			name := properties["GroupName"].(string)
			current := store.Resource(outsideType, name)
			if current == nil {
				current = group(name)
			}
			current["Users"] = append(current["Users"].([]any), outsider)
			store.Put(outsideType, name, current)
			return func() bool {
				return slices.Contains(awsapi.Strings(store.Resource(outsideType, name), "Users"), outsider)
			}
		},
	})
}

func Test_InRegion(t *testing.T) {
	opts := iam.Options{}
	inRegion("eu-west-2")(&opts)
	require.Equal(t, "eu-west-2", opts.Region)
}

func replace(properties ...string) jsondiff.Patch {
	patch := jsondiff.Patch{}
	for _, p := range properties {
		patch = append(patch, jsondiff.Operation{Type: "replace", Path: "/" + p})
	}
	return patch
}

func Test_CreateAndRead(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	ctx := t.Context()

	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: group("admins", "ana", "bo", "ANA", "cy"), ClientToken: token})
	require.NoError(t, err)
	require.Equal(t, "admins|"+native.GenerateName(token), identifier)
	require.Equal(t, []string{"AddUserToGroup", "AddUserToGroup", "AddUserToGroup"}, h.api.calls, "one user per call, once each")

	again, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: group("admins", "ana", "bo", "cy"), ClientToken: token})
	require.NoError(t, err)
	require.Equal(t, identifier, again, "a create sent again has the same identifier")

	t.Run("read returns every user, across pages", func(t *testing.T) {
		props, err := h.Read(ctx, region, identifier)
		require.NoError(t, err)
		require.Equal(t, map[string]any{"Id": identifier, "GroupName": "admins", "Users": []any{"ana", "bo", "cy"}}, props)
	})

	t.Run("read scoped returns the managed users, as the template spells them", func(t *testing.T) {
		props, err := h.ReadScoped(ctx, region, identifier, []map[string]any{group("admins", "Bo"), group("admins", "CY", "zed")})
		require.NoError(t, err)
		require.Equal(t, []any{"Bo", "CY"}, props["Users"])
	})

	t.Run("none of the managed users is in the group", func(t *testing.T) {
		_, err := h.ReadScoped(ctx, region, identifier, []map[string]any{group("admins", "zed")})
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("an empty group is not found", func(t *testing.T) {
		_, err := h.Read(ctx, region, "empty|"+native.GenerateName(token))
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("a missing group is not found", func(t *testing.T) {
		_, err := h.Read(ctx, region, "missing-group|"+native.GenerateName(token))
		require.ErrorIs(t, err, native.ErrNotFound)
	})

	t.Run("an identifier another handler assigned takes the group from the managed properties", func(t *testing.T) {
		props, err := h.ReadScoped(ctx, region, "id-1", []map[string]any{{"Users": []any{"ana"}}, group("admins", "ana")})
		require.NoError(t, err)
		require.Equal(t, "admins", props["GroupName"])
		require.Equal(t, "id-1", props["Id"])
	})

	t.Run("an identifier with no group", func(t *testing.T) {
		_, err := h.Read(ctx, region, "id-1")
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.True(t, failure.ClientError)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
	})

	t.Run("GetGroup fails", func(t *testing.T) {
		h.api.errs["GetGroup"] = fault("Throttling")
		defer delete(h.api.errs, "GetGroup")
		_, err := h.Read(ctx, region, identifier)
		require.ErrorContains(t, err, "Throttling")
		require.NotErrorIs(t, err, native.ErrNotFound)
	})
}

func Test_Create_Fails(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	ctx := t.Context()

	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: group("admins", "missing-user", "bo"), ClientToken: token})
	require.ErrorContains(t, err, "NoSuchEntity", "AWS rejected the first call")
	require.Empty(t, identifier, "nothing was added, so there is no resource to keep")

	identifier, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: group("admins", "ana", "missing-user"), ClientToken: token})
	var failure *native.Failure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, native.OutcomePartial, failure.Outcome, "the first user was added")
	require.True(t, failure.ClientError)
	require.Equal(t, "admins|"+native.GenerateName(token), identifier, "the user added is kept, so a delete removes it")

	for name, properties := range map[string]map[string]any{
		"no group":    group("", "ana"),
		"no users":    group("admins"),
		"no settings": {},
	} {
		t.Run(name, func(t *testing.T) {
			h.api.calls = nil
			identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: properties, ClientToken: token})
			var failure *native.Failure
			require.ErrorAs(t, err, &failure)
			require.True(t, failure.ClientError)
			require.Equal(t, native.OutcomeRejected, failure.Outcome)
			require.Empty(t, identifier)
			require.Empty(t, h.api.calls, "nothing reaches AWS")
		})
	}
}

func Test_Update(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: group("admins", "ana", "bo"), ClientToken: token})
	require.NoError(t, err)
	update := func(current, desired map[string]any) error {
		h.api.calls = nil
		return h.Update(ctx, native.UpdateRequest{Region: region, Identifier: identifier, Current: current, Desired: desired, Patch: replace("Users")})
	}

	t.Run("removals come first, one user per call", func(t *testing.T) {
		require.NoError(t, update(group("admins", "ana", "bo"), group("admins", "bo", "cy", "di")))
		require.Equal(t, []string{"RemoveUserFromGroup", "AddUserToGroup", "AddUserToGroup"}, h.api.calls)
	})

	t.Run("a change only in case changes nothing", func(t *testing.T) {
		require.NoError(t, update(group("admins", "bo", "cy", "di"), group("admins", "BO", "Cy", "di")))
		require.Empty(t, h.api.calls)
	})

	t.Run("a user already removed is not an error", func(t *testing.T) {
		require.NoError(t, update(group("admins", "bo", "cy", "di", "gone"), group("admins", "bo", "cy", "di")))
		require.Equal(t, []string{"RemoveUserFromGroup"}, h.api.calls)
	})

	t.Run("a failed removal is rejected", func(t *testing.T) {
		h.api.errs["RemoveUserFromGroup"] = fault("LimitExceeded")
		defer delete(h.api.errs, "RemoveUserFromGroup")
		err := update(group("admins", "bo", "cy", "di"), group("admins", "bo", "cy", "ed"))
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
		require.Equal(t, native.OutcomeRejected, failure.Outcome)
		require.True(t, failure.ClientError)
	})

	t.Run("an identifier with no group", func(t *testing.T) {
		err := h.Update(ctx, native.UpdateRequest{Region: region, Identifier: "id-1", Current: map[string]any{}, Desired: map[string]any{}})
		var failure *native.Failure
		require.ErrorAs(t, err, &failure)
	})
}

func Test_Delete(t *testing.T) {
	store := awsfake.NewCloudControl()
	h := newTracked(store)
	ctx := t.Context()
	identifier, err := h.Create(ctx, native.CreateRequest{Region: region, Properties: group("admins", "ana", "bo"), ClientToken: token})
	require.NoError(t, err)
	store.Put(outsideType, "admins", group("admins", outsider))

	require.ErrorContains(t, h.Delete(ctx, region, identifier), "DeleteScoped")
	require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, nil), native.ErrNotFound, "no managed users lists none")

	h.api.errs["RemoveUserFromGroup"] = fault("Throttling")
	require.ErrorContains(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{group("admins", "ana", "bo")}), "Throttling")
	delete(h.api.errs, "RemoveUserFromGroup")

	require.NoError(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{group("admins", "ANA", "bo")}))
	require.Nil(t, store.Resource(userToGroupAdditionType, identifier))
	require.Equal(t, []string{outsider}, h.api.users("admins"), "a user added outside Radius stays")

	require.ErrorIs(t, h.DeleteScoped(ctx, region, identifier, []map[string]any{group("admins", "ana")}), native.ErrNotFound)
	require.ErrorIs(t, h.DeleteScoped(ctx, region, "missing-group|x", []map[string]any{group("missing-group", "ana")}), native.ErrNotFound)
}

func Test_ListAndProgress(t *testing.T) {
	h := newTracked(awsfake.NewCloudControl())
	_, err := h.List(t.Context(), region)
	require.ErrorContains(t, err, "can't be listed")

	for _, op := range []native.Operation{native.OperationCreate, native.OperationUpdate, native.OperationDelete} {
		progress, err := h.Progress(t.Context(), region, "admins|x", op)
		require.NoError(t, err)
		require.True(t, progress.Done)
	}
	require.False(t, errors.Is(err, native.ErrNotFound))
}
