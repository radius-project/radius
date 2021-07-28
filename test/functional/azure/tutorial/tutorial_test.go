// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tutorial_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
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
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.StorageStorageAccounts,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
							keys.TagRadiusComponent:   "statestore",
						},

						// We don't validate the table here, because it is created by Dapr
						// We get enough out of just validating the storage account
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "nodeapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, outputresource.TypeKubernetes, workloads.ResourceKindKubernetes, true),
							workloads.LocalIDService:    validation.NewOutputResource(workloads.LocalIDService, outputresource.TypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pythonapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, outputresource.TypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "statestore",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDaprStateStoreAzureStorage: validation.NewOutputResource(workloads.LocalIDDaprStateStoreAzureStorage, outputresource.TypeARM, workloads.ResourceKindDaprStateStoreAzureStorage, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"dapr-hello": {
						validation.NewK8sObjectForComponent("dapr-hello", "nodeapp"),
						validation.NewK8sObjectForComponent("dapr-hello", "pythonapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_TutorialWebApp(t *testing.T) {
	applicationName := "webapp"
	componentNameWebApp := "todoapp"
	componentNameKV := "kv"
	componentNameDB := "db"
	template := "../../../../docs/content/getting-started/tutorial/webapp/code/template.bicep"
	test := azuretest.NewApplicationTest(t, applicationName, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.ManagedIdentityUserAssignedIdentities,
						Tags: map[string]string{
							keys.TagRadiusApplication: applicationName,
							keys.TagRadiusComponent:   componentNameWebApp,
						},
					},
					{
						Type: azresources.DocumentDBDatabaseAccounts,
						Tags: map[string]string{
							keys.TagRadiusApplication: applicationName,
							keys.TagRadiusComponent:   componentNameDB,
						},
						Children: []validation.ExpectedChildResource{
							{
								Type: azresources.DocumentDBDatabaseAccountsMongodDBDatabases,
								Name: componentNameDB,
							},
						},
					},
					{
						Type: azresources.KeyVaultVaults,
						Tags: map[string]string{
							keys.TagRadiusApplication: applicationName,
							keys.TagRadiusComponent:   componentNameKV,
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					applicationName: {
						validation.NewK8sObjectForComponent(applicationName, componentNameWebApp),
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameKV,
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDKeyVault: validation.NewOutputResource(workloads.LocalIDKeyVault, outputresource.TypeARM, workloads.ResourceKindAzureKeyVault, true),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameDB,
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDAzureCosmosDBMongo: validation.NewOutputResource(workloads.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, workloads.ResourceKindAzureCosmosDBMongo, true),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameWebApp,
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment:                    validation.NewOutputResource(workloads.LocalIDDeployment, outputresource.TypeKubernetes, workloads.ResourceKindKubernetes, true),
							workloads.LocalIDService:                       validation.NewOutputResource(workloads.LocalIDService, outputresource.TypeKubernetes, workloads.ResourceKindKubernetes, true),
							workloads.LocalIDUserAssignedManagedIdentityKV: validation.NewOutputResource(workloads.LocalIDUserAssignedManagedIdentityKV, outputresource.TypeARM, workloads.ResourceKindAzureUserAssignedManagedIdentity, true),
							workloads.LocalIDRoleAssignmentKVKeys:          validation.NewOutputResource(workloads.LocalIDRoleAssignmentKVKeys, outputresource.TypeARM, workloads.ResourceKindAzureRoleAssignment, true),
							workloads.LocalIDRoleAssignmentKVSecretsCerts:  validation.NewOutputResource(workloads.LocalIDRoleAssignmentKVSecretsCerts, outputresource.TypeARM, workloads.ResourceKindAzureRoleAssignment, true),
							workloads.LocalIDAADPodIdentity:                validation.NewOutputResource(workloads.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, workloads.ResourceKindAzurePodIdentity, true),
							workloads.LocalIDKeyVaultSecret:                validation.NewOutputResource(workloads.LocalIDKeyVaultSecret, outputresource.TypeARM, workloads.ResourceKindAzureKeyVaultSecret, true),
						},
					},
				},
			},
		},
	})

	test.Test(t)
}
