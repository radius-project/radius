// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func Test_KeyVaultManaged(t *testing.T) {
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
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDKeyVault: validation.NewOutputResource(outputresource.LocalIDKeyVault, outputresource.TypeARM, resourcekinds.KindAzureKeyVault, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "kvaccessor",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment:                    validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentityKV: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentityKV, outputresource.TypeARM, resourcekinds.KindAzureUserAssignedManagedIdentity, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDRoleAssignmentKVKeys:          validation.NewOutputResource(outputresource.LocalIDRoleAssignmentKVKeys, outputresource.TypeARM, resourcekinds.KindAzureRoleAssignment, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDRoleAssignmentKVSecretsCerts:  validation.NewOutputResource(outputresource.LocalIDRoleAssignmentKVSecretsCerts, outputresource.TypeARM, resourcekinds.KindAzureRoleAssignment, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAADPodIdentity:                validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, resourcekinds.KindAzurePodIdentity, true, false, rest.OutputResourceStatus{}),
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
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				appclient := radclient.NewApplicationClient(at.Options.ARMConnection, at.Options.Environment.SubscriptionID)

				// get application and verify name
				response, err := appclient.Get(ctx, at.Options.Environment.ResourceGroup, application, nil)
				require.NoError(t, err)
				assert.Equal(t, application, *response.ApplicationResource.Name)
			},
		},
	})

	test.Test(t)
}
