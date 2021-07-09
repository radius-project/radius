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
							workloads.LocalIDKeyVault: validation.NewOutputResource(workloads.LocalIDKeyVault, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureKeyVault, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "kvaccessor",
						OutputResources: map[string]validation.OutputResourceSet{
							workloads.LocalIDDeployment:                    validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							workloads.LocalIDUserAssignedManagedIdentityKV: validation.NewOutputResource(workloads.LocalIDUserAssignedManagedIdentityKV, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureUserAssignedManagedIdentity, true),
							workloads.LocalIDRoleAssignmentKVKeys:          validation.NewOutputResource(workloads.LocalIDRoleAssignmentKVKeys, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							workloads.LocalIDRoleAssignmentKVSecretsCerts:  validation.NewOutputResource(workloads.LocalIDRoleAssignmentKVSecretsCerts, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							workloads.LocalIDAADPodIdentity:                validation.NewOutputResource(workloads.LocalIDAADPodIdentity, workloads.OutputResourceTypePodIdentity, workloads.ResourceKindAzurePodIdentity, true),
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
