// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package outputresource

import (
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/stretchr/testify/require"
)

func TestGetDependencies(t *testing.T) {
	outputResource, _ := getTestOutputResourceWithDependencies()

	dependencies, err := outputResource.GetDependencies()
	require.NoError(t, err)
	require.Equal(t, []string{LocalIDUserAssignedManagedIdentity, LocalIDRoleAssignmentKVKeys},
		dependencies)
}

func TestGetDependencies_MissingLocalID(t *testing.T) {
	testResource1 := OutputResource{
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRoleAssignment,
			Provider: providers.ProviderAzure,
		},
	}

	testResource2 := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRoleAssignment,
			Provider: providers.ProviderAzure,
		},
		Dependencies: []Dependency{{LocalID: testResource1.LocalID}},
	}

	_, err := testResource2.GetDependencies()
	expectedErrorMsg := "missing localID for outputresource"
	require.EqualError(t, err, expectedErrorMsg)
}

func TestGetDependencies_Empty(t *testing.T) {
	testOutputResource := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureUserAssignedManagedIdentity,
			Provider: providers.ProviderAzure,
		},
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
		outputResourcesMap[LocalIDAADPodIdentity]}
	require.Equal(t, expected, ordered)
}

// Returns output resource with multiple dependencies and a map of localID/unordered list of output resources
func getTestOutputResourceWithDependencies() (OutputResource, map[string]OutputResource) {
	managedIdentity := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureUserAssignedManagedIdentity,
			Provider: providers.ProviderAzure,
		},
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRoleAssignment,
			Provider: providers.ProviderAzure,
		},
		Dependencies: []Dependency{{LocalID: managedIdentity.LocalID}},
	}

	aadPodIdentity := OutputResource{
		LocalID: LocalIDAADPodIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzurePodIdentity,
			Provider: providers.ProviderAzureKubernetesService,
		},
		Dependencies: []Dependency{
			{LocalID: managedIdentity.LocalID},
			{LocalID: roleAssignmentKeys.LocalID},
		},
	}

	outputResources := map[string]OutputResource{
		LocalIDAADPodIdentity:              aadPodIdentity,
		LocalIDUserAssignedManagedIdentity: managedIdentity,
		LocalIDRoleAssignmentKVKeys:        roleAssignmentKeys,
	}

	return aadPodIdentity, outputResources
}
