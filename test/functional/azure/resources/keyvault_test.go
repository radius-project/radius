// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/workloads"
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
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "kv",
						OutputResources: map[string]validation.OutputResourceSet{
							"KeyVault": validation.NewOutputResource("KeyVault", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureKeyVault, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "kvaccessor",
						OutputResources: map[string]validation.OutputResourceSet{
							"Deployment":                     validation.NewOutputResource("Deployment", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							"UserAssignedManagedIdentity-KV": validation.NewOutputResource("UserAssignedManagedIdentity-KV", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureUserAssignedManagedIdentity, true),
							"RoleAssignment-KVKeys":          validation.NewOutputResource("RoleAssignment-KVKeys", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							"RoleAssignment-KVSecretsCerts":  validation.NewOutputResource("RoleAssignment-KVSecretsCerts", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							"AADPodIdentity":                 validation.NewOutputResource("AADPodIdentity", workloads.OutputResourceTypePodIdentity, workloads.ResourceKindAzurePodIdentity, true),
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
			SkipARMResources: true,
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
