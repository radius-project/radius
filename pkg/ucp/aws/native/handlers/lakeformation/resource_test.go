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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
)

func newResource(stored map[string]any) (*Resource, *fakeAPI) {
	store := awsfake.NewCloudControl()
	if stored != nil {
		store.Put(resourceType, dataArn, stored)
	}
	api := newFakeAPI(store)
	return NewResource(api), api
}

func Test_Resource_Update(t *testing.T) {
	tests := []struct {
		name    string
		stored  map[string]any
		desired map[string]any
		removed []string
		calls   []string
		want    map[string]any
	}{
		{
			name:    "from the service-linked role to another role",
			stored:  map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true},
			desired: map[string]any{"ResourceArn": dataArn, "RoleArn": roleArn, "UseServiceLinkedRole": false},
			calls:   []string{"DescribeResource", "UpdateResource"},
			want:    map[string]any{"RoleArn": roleArn},
		},
		{
			name:    "to the service-linked role",
			stored:  map[string]any{"ResourceArn": dataArn, "RoleArn": roleArn, "HybridAccessEnabled": true},
			desired: map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true, "HybridAccessEnabled": true},
			removed: []string{"RoleArn"},
			calls:   []string{"DescribeResource", "DeregisterResource", "RegisterResource"},
			want:    map[string]any{"RoleArn": slrArn, "HybridAccessEnabled": true},
		},
		{
			name:    "removed values",
			stored:  map[string]any{"ResourceArn": dataArn, "RoleArn": roleArn, "HybridAccessEnabled": true, "WithFederation": true},
			desired: map[string]any{"ResourceArn": dataArn},
			removed: []string{"RoleArn", "HybridAccessEnabled", "WithFederation"},
			calls:   []string{"DescribeResource", "UpdateResource"},
			want:    map[string]any{"RoleArn": roleArn, "HybridAccessEnabled": false, "WithFederation": false},
		},
		{
			name:    "a role while the service-linked role is on",
			stored:  map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true},
			desired: map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true, "RoleArn": otherRole, "WithFederation": true},
			calls:   []string{"DescribeResource", "UpdateResource"},
			want:    map[string]any{"RoleArn": slrArn, "WithFederation": true},
		},
		{
			name:    "only the service-linked role turned off",
			stored:  map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true},
			desired: map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": false},
			calls:   []string{"DescribeResource"},
			want:    map[string]any{"RoleArn": slrArn},
		},
		{
			name:    "the service-linked role it already has",
			stored:  map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true},
			desired: map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true},
			calls:   []string{"DescribeResource"},
			want:    map[string]any{"RoleArn": slrArn},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			h, api := newResource(tt.stored)
			current, err := h.Read(ctx, region, dataArn)
			require.NoError(t, err)
			api.takeCalls()
			err = h.Update(ctx, native.UpdateRequest{Region: region, Identifier: dataArn, Current: current, Desired: tt.desired, Patch: patchOf(t, current, tt.desired, tt.removed...)})
			require.NoError(t, err)
			require.Equal(t, tt.calls, api.takeCalls())
			got, err := h.Read(ctx, region, dataArn)
			require.NoError(t, err)
			for k, v := range tt.want {
				require.Equal(t, v, got[k], k)
			}
		})
	}
}

func Test_Resource_Errors(t *testing.T) {
	ctx := t.Context()
	h, api := newResource(nil)
	_, err := h.Read(ctx, region, dataArn)
	require.ErrorIs(t, err, native.ErrNotFound)
	require.ErrorIs(t, h.Update(ctx, native.UpdateRequest{Region: region, Identifier: dataArn}), native.ErrNotFound)
	require.ErrorIs(t, h.Delete(ctx, region, dataArn), native.ErrNotFound)

	_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"ResourceArn": 5}})
	require.ErrorContains(t, err, "failed to read the properties")
	_, err = h.Create(ctx, native.CreateRequest{Region: region, Properties: map[string]any{"ResourceArn": dataArn}})
	require.ErrorContains(t, err, "InvalidInputException", "AWS needs a role or the service-linked role")

	failure := nativefake.ServerError("InternalServiceException", "failed")
	api.errs["ListResources"] = failure
	_, err = h.List(ctx, region)
	require.ErrorIs(t, err, failure)

	h, api = newResource(map[string]any{"ResourceArn": dataArn, "UseServiceLinkedRole": true})
	api.empty = true
	_, err = h.Read(ctx, region, dataArn)
	require.ErrorIs(t, err, native.ErrNotFound)
}
