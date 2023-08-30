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

package v1

import (
	"testing"

	"github.com/project-radius/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/project-radius/radius/pkg/ucp/resources/kubernetes"
	"github.com/stretchr/testify/require"
)

func TestGetDependencies(t *testing.T) {
	outputResource, _ := getTestOutputResourceWithDependencies()

	dependencies, err := outputResource.GetDependencies()
	require.NoError(t, err)
	require.Equal(t, []string{LocalIDUserAssignedManagedIdentity, LocalIDRoleAssignmentKVKeys}, dependencies)
}

func TestExistDependency(t *testing.T) {
	testResource := &Resource{
		Dependencies: []string{LocalIDSecret},
	}

	require.True(t, testResource.ExistDependency(LocalIDSecret))
	require.False(t, testResource.ExistDependency(LocalIDDeployment))
}

func TestGetDependencies_MissingLocalID(t *testing.T) {
	testResource1 := OutputResource{}

	testResource2 := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		CreateResource: &Resource{
			Dependencies: []string{testResource1.LocalID},
		},
	}

	_, err := testResource2.GetDependencies()
	expectedErrorMsg := "missing localID for outputresource"
	require.EqualError(t, err, expectedErrorMsg)
}

func TestGetDependencies_Empty(t *testing.T) {
	testOutputResource := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
	}

	dependencies, err := testOutputResource.GetDependencies()
	require.NoError(t, err)
	require.Empty(t, dependencies)
}

func TestOrderOutputResources(t *testing.T) {
	_, outputResourcesMap := getTestOutputResourceWithDependencies()
	outputResources := []OutputResource{}
	for _, resource := range outputResourcesMap {
		outputResources = append(outputResources, resource)
	}
	ordered, err := OrderOutputResources(outputResources)
	require.NoError(t, err)

	expected := []OutputResource{outputResourcesMap[LocalIDUserAssignedManagedIdentity], outputResourcesMap[LocalIDRoleAssignmentKVKeys],
		outputResourcesMap[LocalIDFederatedIdentity]}
	require.Equal(t, expected, ordered)
}

// Returns output resource with multiple dependencies and a map of localID/unordered list of output resources
func getTestOutputResourceWithDependencies() (OutputResource, map[string]OutputResource) {
	managedIdentity := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		CreateResource: &Resource{
			Dependencies: []string{managedIdentity.LocalID},
		},
	}

	federatedIdentity := OutputResource{
		LocalID: LocalIDFederatedIdentity,
		CreateResource: &Resource{
			Dependencies: []string{managedIdentity.LocalID, roleAssignmentKeys.LocalID},
		},
	}

	outputResources := map[string]OutputResource{
		LocalIDFederatedIdentity:           federatedIdentity,
		LocalIDUserAssignedManagedIdentity: managedIdentity,
		LocalIDRoleAssignmentKVKeys:        roleAssignmentKeys,
	}

	return federatedIdentity, outputResources
}

func TestGetGCOutputResources_Same(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	managedIdentity := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi"),
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.Authorization/roleAssignments/test-ra"),
	}

	federatedIdentity := OutputResource{
		LocalID: LocalIDFederatedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi/federatedIdentities/test-fi"),
	}

	after = append(after, managedIdentity)
	after = append(after, roleAssignmentKeys)
	after = append(after, federatedIdentity)

	before = append(before, managedIdentity)
	before = append(before, roleAssignmentKeys)
	before = append(before, federatedIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{}, diff)
}

func TestGetGCOutputResources_ResourceDiffersByID(t *testing.T) {
	after := []OutputResource{
		{
			LocalID: LocalIDRoleAssignmentKVKeys,
			ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/after"),
		},
	}
	before := []OutputResource{
		{
			LocalID: LocalIDRoleAssignmentKVKeys,
			ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/before"),
		},
	}

	diff := GetGCOutputResources(after, before)
	require.Equal(t, before, diff)
}

func TestGetGCOutputResources_SameWithAdditionalOutputResource(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	managedIdentity := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi"),
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.Authorization/roleAssignments/test-ra"),
	}

	federatedIdentity := OutputResource{
		LocalID: LocalIDFederatedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi/federatedIdentities/test-fi"),
	}

	after = append(after, managedIdentity)
	after = append(after, roleAssignmentKeys)
	after = append(after, federatedIdentity)

	before = append(before, roleAssignmentKeys)
	before = append(before, federatedIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{}, diff)
}

func TestGetGCOutputResources_ManagedIdentityShouldBeDeleted(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	managedIdentity := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi"),
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.Authorization/roleAssignments/test-ra"),
	}

	federatedIdentity := OutputResource{
		LocalID: LocalIDFederatedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi/federatedIdentities/test-fi"),
	}

	after = append(after, roleAssignmentKeys)
	after = append(after, federatedIdentity)

	before = append(before, managedIdentity)
	before = append(before, roleAssignmentKeys)
	before = append(before, federatedIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{managedIdentity}, diff)
}

func TestGetGCOutputResources_ALotOfResources(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	managedIdentity1 := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi1"),
	}

	managedIdentity2 := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi2"),
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.Authorization/roleAssignments/test-ra"),
	}

	federatedIdentity := OutputResource{
		LocalID: LocalIDFederatedIdentity,
		ID:      resources.MustParse("/subscriptions/test-subscription/resourceGroups/test-rg/providers/Microsoft.ManagedIdentity/userAssignedIdentities/test-mi/federatedIdentities/test-fi"),
	}

	after = append(after, managedIdentity1)
	after = append(after, roleAssignmentKeys)
	after = append(after, federatedIdentity)

	before = append(before, managedIdentity1)
	before = append(before, managedIdentity2)
	before = append(before, roleAssignmentKeys)
	before = append(before, federatedIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{managedIdentity2}, diff)
}

func TestGetGCOutputResources_ResourceRemoved(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	deployment := OutputResource{
		LocalID: LocalIDDeployment,
		ID: resources_kubernetes.IDFromParts(
			resources_kubernetes.PlaneNameTODO,
			"",
			resources_kubernetes.KindDeployment,
			"test-namespace",
			"test-deployment"),
	}

	secret := OutputResource{
		LocalID: LocalIDSecret,
		ID: resources_kubernetes.IDFromParts(
			resources_kubernetes.PlaneNameTODO,
			"",
			resources_kubernetes.KindSecret,
			"test-namespace",
			"test-secret"),
	}

	after = append(after, deployment)

	before = append(before, secret)
	before = append(before, deployment)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{secret}, diff)

}
