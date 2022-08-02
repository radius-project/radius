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

func TestGetGCOutputResources_Same(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

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

	after = append(after, managedIdentity)
	after = append(after, roleAssignmentKeys)
	after = append(after, aadPodIdentity)

	before = append(before, managedIdentity)
	before = append(before, roleAssignmentKeys)
	before = append(before, aadPodIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{}, diff)
}

func TestGetGCOutputResources_SameWithAdditionalOutputResource(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

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

	after = append(after, managedIdentity)
	after = append(after, roleAssignmentKeys)
	after = append(after, aadPodIdentity)

	before = append(before, roleAssignmentKeys)
	before = append(before, aadPodIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{}, diff)
}

func TestGetGCOutputResources_ManagedIdentityShouldBeDeleted(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

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

	after = append(after, roleAssignmentKeys)
	after = append(after, aadPodIdentity)

	before = append(before, managedIdentity)
	before = append(before, roleAssignmentKeys)
	before = append(before, aadPodIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{managedIdentity}, diff)
}

func TestGetGCOutputResources_ALotOfResources(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	managedIdentity1 := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureUserAssignedManagedIdentity,
			Provider: providers.ProviderAzure,
		},
	}

	managedIdentity2 := OutputResource{
		LocalID: LocalIDUserAssignedManagedIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type: resourcekinds.AzureUserAssignedManagedIdentity,
			// Fixme: Kubernetes is not possible?
			Provider: providers.ProviderKubernetes,
		},
	}

	roleAssignmentKeys := OutputResource{
		LocalID: LocalIDRoleAssignmentKVKeys,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureRoleAssignment,
			Provider: providers.ProviderAzure,
		},
		Dependencies: []Dependency{{LocalID: managedIdentity1.LocalID}},
	}

	aadPodIdentity := OutputResource{
		LocalID: LocalIDAADPodIdentity,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzurePodIdentity,
			Provider: providers.ProviderAzureKubernetesService,
		},
		Dependencies: []Dependency{
			{LocalID: managedIdentity1.LocalID},
			{LocalID: roleAssignmentKeys.LocalID},
		},
	}

	after = append(after, managedIdentity1)
	after = append(after, roleAssignmentKeys)
	after = append(after, aadPodIdentity)

	before = append(before, managedIdentity1)
	before = append(before, managedIdentity2)
	before = append(before, roleAssignmentKeys)
	before = append(before, aadPodIdentity)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{managedIdentity2}, diff)
}

func TestGetGCOutputResources_Secret(t *testing.T) {
	after := []OutputResource{}
	before := []OutputResource{}

	deployment := OutputResource{
		LocalID: LocalIDDeployment,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
		},
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.Deployment,
			Provider: providers.ProviderKubernetes,
		},
		Dependencies: nil,
	}

	secret := OutputResource{
		LocalID: LocalIDSecret,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.Secret,
				Provider: providers.ProviderKubernetes,
			},
		},
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.Secret,
			Provider: providers.ProviderKubernetes,
		},
		Dependencies: nil,
	}

	after = append(after, deployment)

	before = append(before, secret)
	before = append(before, deployment)

	diff := GetGCOutputResources(after, before)

	require.Equal(t, []OutputResource{secret}, diff)

}
