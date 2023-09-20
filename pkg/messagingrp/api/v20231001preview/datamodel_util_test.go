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

package v20231001preview

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

func TestToProvisioningStateDataModel(t *testing.T) {
	stateTests := []struct {
		versioned ProvisioningState
		datamodel v1.ProvisioningState
	}{
		{ProvisioningStateUpdating, v1.ProvisioningStateUpdating},
		{ProvisioningStateDeleting, v1.ProvisioningStateDeleting},
		{ProvisioningStateAccepted, v1.ProvisioningStateAccepted},
		{ProvisioningStateSucceeded, v1.ProvisioningStateSucceeded},
		{ProvisioningStateFailed, v1.ProvisioningStateFailed},
		{ProvisioningStateCanceled, v1.ProvisioningStateCanceled},
		{"", v1.ProvisioningStateAccepted},
	}

	for _, tt := range stateTests {
		sc := toProvisioningStateDataModel(&tt.versioned)
		require.Equal(t, tt.datamodel, sc)
	}
}

func TestFromProvisioningStateDataModel(t *testing.T) {
	testCases := []struct {
		datamodel v1.ProvisioningState
		versioned ProvisioningState
	}{
		{v1.ProvisioningStateUpdating, ProvisioningStateUpdating},
		{v1.ProvisioningStateDeleting, ProvisioningStateDeleting},
		{v1.ProvisioningStateAccepted, ProvisioningStateAccepted},
		{v1.ProvisioningStateSucceeded, ProvisioningStateSucceeded},
		{v1.ProvisioningStateFailed, ProvisioningStateFailed},
		{v1.ProvisioningStateCanceled, ProvisioningStateCanceled},
		{"", ProvisioningStateAccepted},
	}

	for _, testCase := range testCases {
		sc := fromProvisioningStateDataModel(testCase.datamodel)
		require.Equal(t, testCase.versioned, *sc)
	}
}

func TestFromSystemDataModel(t *testing.T) {
	systemDataTests := []v1.SystemData{
		{
			CreatedBy:          "",
			CreatedByType:      "",
			CreatedAt:          "",
			LastModifiedBy:     "",
			LastModifiedByType: "",
			LastModifiedAt:     "",
		}, {
			CreatedBy:          "fakeid@live.com",
			CreatedByType:      "",
			CreatedAt:          "2021-09-24T19:09:00Z",
			LastModifiedBy:     "fakeid@live.com",
			LastModifiedByType: "",
			LastModifiedAt:     "2021-09-25T19:09:00Z",
		}, {
			CreatedBy:          "fakeid@live.com",
			CreatedByType:      "User",
			CreatedAt:          "2021-09-24T19:09:00Z",
			LastModifiedBy:     "fakeid@live.com",
			LastModifiedByType: "User",
			LastModifiedAt:     "2021-09-25T19:09:00Z",
		},
	}

	for _, tt := range systemDataTests {
		versioned := fromSystemDataModel(tt)
		require.Equal(t, tt.CreatedBy, string(*versioned.CreatedBy))
		require.Equal(t, tt.CreatedByType, string(*versioned.CreatedByType))
		c, err := versioned.CreatedAt.MarshalText()
		require.NoError(t, err)
		if tt.CreatedAt == "" {
			tt.CreatedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.CreatedAt, string(c))

		require.Equal(t, tt.LastModifiedBy, string(*versioned.LastModifiedBy))
		require.Equal(t, tt.LastModifiedByType, string(*versioned.LastModifiedByType))
		c, err = versioned.LastModifiedAt.MarshalText()
		require.NoError(t, err)
		if tt.LastModifiedAt == "" {
			tt.LastModifiedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.LastModifiedAt, string(c))
	}
}

func TestToRecipeDataModel(t *testing.T) {
	testset := []struct {
		versioned *Recipe
		datamodel portableresources.ResourceRecipe
	}{
		{
			nil,
			portableresources.ResourceRecipe{
				Name: portableresources.DefaultRecipeName,
			},
		},
		{
			&Recipe{
				Name: to.Ptr("test"),
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
			portableresources.ResourceRecipe{
				Name: "test",
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
		},
		{
			&Recipe{
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
			portableresources.ResourceRecipe{
				Name: portableresources.DefaultRecipeName,
				Parameters: map[string]any{
					"foo": "bar",
				},
			},
		},
	}
	for _, testCase := range testset {
		sc := toRecipeDataModel(testCase.versioned)
		require.Equal(t, testCase.datamodel, sc)
	}
}

func TestFromResourcesDataModel(t *testing.T) {
	testset := []struct {
		DMResources        []*portableresources.ResourceReference
		VersionedResources []*ResourceReference
	}{
		{
			DMResources:        []*portableresources.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			VersionedResources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
		{
			DMResources:        []*portableresources.ResourceReference{},
			VersionedResources: []*ResourceReference{},
		},
	}

	for _, tt := range testset {
		versioned := fromResourcesDataModel(tt.DMResources)
		require.Equal(t, tt.VersionedResources, versioned)

	}
}

func TestToResourcesDataModel(t *testing.T) {
	testset := []struct {
		DMResources        []*portableresources.ResourceReference
		VersionedResources []*ResourceReference
	}{
		{
			DMResources:        []*portableresources.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			VersionedResources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
		{
			DMResources:        []*portableresources.ResourceReference{},
			VersionedResources: []*ResourceReference{},
		},
	}

	for _, tt := range testset {
		dm := toResourcesDataModel(tt.VersionedResources)
		require.Equal(t, tt.DMResources, dm)

	}
}
