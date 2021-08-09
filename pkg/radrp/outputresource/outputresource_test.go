// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetDependencies(t *testing.T) {
	outputResource, _ := getTestOutputResourceWithDependencies()

	dependencies, err := outputResource.GetDependencies()
	require.NoError(t, err)
	require.Equal(t, []string{LocalIDUserAssignedManagedIdentityKV, LocalIDRoleAssignmentKVKeys},
		dependencies)
}

func TestGetDependencies_MissingLocalID(t *testing.T) {
	testResource1 := OutputResource{
		Type: TypeARM,
		Kind: KindAzureRoleAssignment,
	}

	testResource2 := OutputResource{
		LocalID:      LocalIDRoleAssignmentKVKeys,
		Type:         TypeARM,
		Kind:         KindAzureRoleAssignment,
		Dependencies: []OutputResource{testResource1},
	}

	_, err := testResource2.GetDependencies()
	expectedErrorMsg := fmt.Sprintf("missing localID for outputresource kind: %s", KindAzureRoleAssignment)
	require.EqualError(t, err, expectedErrorMsg)
}

func TestGetDependencies_Empty(t *testing.T) {
	testOutputResource := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentityKV,
		Type:    TypeARM,
		Kind:    KindAzureUserAssignedManagedIdentity,
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

	expected := []OutputResource{outputResourcesMap[LocalIDUserAssignedManagedIdentityKV], outputResourcesMap[LocalIDRoleAssignmentKVKeys],
		outputResourcesMap[LocalIDAADPodIdentity]}
	require.Equal(t, expected, ordered)
}

// Returns output resource with multiple dependencies and a map of localID/unordered list of output resources
func getTestOutputResourceWithDependencies() (OutputResource, map[string]OutputResource) {
	managedIdentity := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentityKV,
		Type:    TypeARM,
		Kind:    KindAzureUserAssignedManagedIdentity,
	}

	roleAssignmentKeys := OutputResource{
		LocalID:      LocalIDRoleAssignmentKVKeys,
		Type:         TypeARM,
		Kind:         KindAzureRoleAssignment,
		Dependencies: []OutputResource{managedIdentity},
	}

	aadPodIdentity := OutputResource{
		LocalID:      LocalIDAADPodIdentity,
		Type:         TypeAADPodIdentity,
		Kind:         KindAzurePodIdentity,
		Dependencies: []OutputResource{managedIdentity, roleAssignmentKeys},
	}

	outputResources := map[string]OutputResource{
		LocalIDAADPodIdentity:                aadPodIdentity,
		LocalIDUserAssignedManagedIdentityKV: managedIdentity,
		LocalIDRoleAssignmentKVKeys:          roleAssignmentKeys,
	}

	return aadPodIdentity, outputResources
}
