// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tutorial_test

import (
	"testing"

	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_TutorialDaprMicroservices(t *testing.T) {
	application := "dapr-hello"
	template := "../../../../docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"dapr-hello": {
						validation.NewK8sObjectForComponent("dapr-hello", "nodeapp"),
						validation.NewK8sObjectForComponent("dapr-hello", "pythonapp"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
		},
	})

	test.Test(t)
}

func Test_TutorialWebApp(t *testing.T) {
	applicationName := "webapp"
	template := "../../../../docs/content/getting-started/tutorial/webapp/code/template.bicep"
	test := azuretest.NewApplicationTest(t, applicationName, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					applicationName: {
						validation.NewK8sObjectForComponent(applicationName, "todoapp"),
					},
				},
			},
			SkipARMResources: true,
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: applicationName,
						ComponentName:   "kv",
						OutputResources: map[string]validation.OutputResourceSet{
							workloads.LocalIDKeyVault: validation.NewOutputResource(workloads.LocalIDKeyVault, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureKeyVault, true),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   "db",
						OutputResources: map[string]validation.OutputResourceSet{
							workloads.LocalIDAzureCosmosDBMongo: validation.NewOutputResource(workloads.LocalIDAzureCosmosDBMongo, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureCosmosDBMongo, true),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   "todoapp",
						OutputResources: map[string]validation.OutputResourceSet{
							workloads.LocalIDDeployment:                    validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							workloads.LocalIDService:                       validation.NewOutputResource(workloads.LocalIDService, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							workloads.LocalIDUserAssignedManagedIdentityKV: validation.NewOutputResource(workloads.LocalIDUserAssignedManagedIdentityKV, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureUserAssignedManagedIdentity, true),
							workloads.LocalIDRoleAssignmentKVKeys:          validation.NewOutputResource(workloads.LocalIDRoleAssignmentKVKeys, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							workloads.LocalIDRoleAssignmentKVSecretsCerts:  validation.NewOutputResource(workloads.LocalIDRoleAssignmentKVSecretsCerts, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							workloads.LocalIDAADPodIdentity:                validation.NewOutputResource(workloads.LocalIDAADPodIdentity, workloads.OutputResourceTypePodIdentity, workloads.ResourceKindAzurePodIdentity, true),
							workloads.LocalIDKeyVaultSecret:                validation.NewOutputResource(workloads.LocalIDKeyVaultSecret, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureKeyVaultSecret, true),
						},
					},
				},
			},
		},
	})

	test.Test(t)
}
