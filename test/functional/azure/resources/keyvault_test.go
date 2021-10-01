// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/keyvaultv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_KeyVaultManaged(t *testing.T) {
	t.Skip("Currently in PR")

	application := "azure-resources-keyvault-managed"
	template := "testdata/azure-resources-keyvault-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.ManagedIdentityUserAssignedIdentities,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
							keys.TagRadiusComponent:   "kvaccessor",
						},
					},
					{
						Type: azresources.KeyVaultVaults,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
							keys.TagRadiusComponent:   "kv",
						},
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "kv",
						ResourceType:    keyvaultv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDKeyVault: validation.NewOutputResource(outputresource.LocalIDKeyVault, outputresource.TypeARM, resourcekinds.AzureKeyVault, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "kvaccessor",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment:                  validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:                      validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentity: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentity, outputresource.TypeARM, resourcekinds.AzureUserAssignedManagedIdentity, true, false, rest.OutputResourceStatus{}),
							"role-assignment-1": {
								SkipLocalIDWhenMatching: true,
								OutputResourceType:      outputresource.TypeARM,
								ResourceKind:            resourcekinds.AzureRoleAssignment,
								Managed:                 true,
								VerifyStatus:            false,
							},
							"role-assignment-2": {
								SkipLocalIDWhenMatching: true,
								OutputResourceType:      outputresource.TypeARM,
								ResourceKind:            resourcekinds.AzureRoleAssignment,
								Managed:                 true,
								VerifyStatus:            false,
							},
							outputresource.LocalIDAADPodIdentity: validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, resourcekinds.AzurePodIdentity, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "kvaccessor"),
					},
				},
			},
		},
	})

	test.Test(t)
}
