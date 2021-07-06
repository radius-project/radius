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
	application := "webapp"
	template := "../../../../docs/content/getting-started/tutorial/webapp/code/template.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"webapp": {
						validation.NewK8sObjectForComponent("webapp", "todoapp"),
					},
				},
			},
			SkipARMResources: true,
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
						ComponentName:   "db",
						OutputResources: map[string]validation.OutputResourceSet{
							"AzureCosmosDBMongo": validation.NewOutputResource("AzureCosmosDBMongo", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureCosmosDBMongo, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "todoapp",
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
		},
	})

	test.Test(t)
}
