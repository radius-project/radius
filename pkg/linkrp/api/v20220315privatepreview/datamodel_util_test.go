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

package v20220315privatepreview

import (
	"fmt"
	"testing"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/to"

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

func TestUnmarshalTimeString(t *testing.T) {
	parsedTime := unmarshalTimeString("2021-09-24T19:09:00.000000Z")
	require.NotNil(t, parsedTime)

	require.Equal(t, 2021, parsedTime.Year())
	require.Equal(t, time.Month(9), parsedTime.Month())
	require.Equal(t, 24, parsedTime.Day())

	parsedTime = unmarshalTimeString("")
	require.NotNil(t, parsedTime)
	require.Equal(t, 1, parsedTime.Year())
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
		c, _ := versioned.CreatedAt.MarshalText()
		if tt.CreatedAt == "" {
			tt.CreatedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.CreatedAt, string(c))

		require.Equal(t, tt.LastModifiedBy, string(*versioned.LastModifiedBy))
		require.Equal(t, tt.LastModifiedByType, string(*versioned.LastModifiedByType))
		c, _ = versioned.LastModifiedAt.MarshalText()
		if tt.LastModifiedAt == "" {
			tt.LastModifiedAt = "0001-01-01T00:00:00Z"
		}
		require.Equal(t, tt.LastModifiedAt, string(c))
	}
}

func TestToResourcesDataModel(t *testing.T) {
	testset := []struct {
		DMResources        []*linkrp.ResourceReference
		VersionedResources []*ResourceReference
	}{
		{
			DMResources:        []*linkrp.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			VersionedResources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
		{
			DMResources:        []*linkrp.ResourceReference{},
			VersionedResources: []*ResourceReference{},
		},
	}

	for _, tt := range testset {
		dm := toResourcesDataModel(tt.VersionedResources)
		require.Equal(t, tt.DMResources, dm)

	}
}

func TestFromResourcesDataModel(t *testing.T) {
	testset := []struct {
		DMResources        []*linkrp.ResourceReference
		VersionedResources []*ResourceReference
	}{
		{
			DMResources:        []*linkrp.ResourceReference{{ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"}, {ID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1"}},
			VersionedResources: []*ResourceReference{{ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache")}, {ID: to.Ptr("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache1")}},
		},
		{
			DMResources:        []*linkrp.ResourceReference{},
			VersionedResources: []*ResourceReference{},
		},
	}

	for _, tt := range testset {
		versioned := fromResourcesDataModel(tt.DMResources)
		require.Equal(t, tt.VersionedResources, versioned)

	}
}

func TestToResourceProvisiongDataModel(t *testing.T) {
	testset := []struct {
		versioned ResourceProvisioning
		datamodel linkrp.ResourceProvisioning
		err       error
	}{
		{
			ResourceProvisioningManual,
			linkrp.ResourceProvisioningManual,
			nil,
		},
		{
			ResourceProvisioningRecipe,
			linkrp.ResourceProvisioningRecipe,
			nil,
		},
		{
			"",
			"",
			&v1.ErrModelConversion{
				PropertyName: "$.properties.resourceProvisioning",
				ValidValue:   fmt.Sprintf("one of %s", PossibleResourceProvisioningValues()),
			},
		},
	}
	for _, tt := range testset {
		sc, err := toResourceProvisiongDataModel(&tt.versioned)

		if tt.err != nil {
			require.EqualError(t, err, tt.err.Error())
			continue
		}

		require.NoError(t, err)
		require.Equal(t, tt.datamodel, sc)
		require.NoError(t, err)
	}
}

func TestFromResourceProvisiongDataModel(t *testing.T) {
	testCases := []struct {
		datamodel linkrp.ResourceProvisioning
		versioned ResourceProvisioning
	}{
		{linkrp.ResourceProvisioningManual, ResourceProvisioningManual},
		{linkrp.ResourceProvisioningRecipe, ResourceProvisioningRecipe},
		{"", ResourceProvisioningRecipe},
	}

	for _, testCase := range testCases {
		sc := fromResourceProvisioningDataModel(testCase.datamodel)
		require.Equal(t, testCase.versioned, *sc)
	}
}
