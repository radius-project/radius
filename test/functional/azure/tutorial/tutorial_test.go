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

			// Health has not yet been implemented
			SkipOutputResourceStatus: true,

			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "nodeapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pythonapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "statestore",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDaprStateStoreAzureStorage: validation.NewOutputResource(outputresource.LocalIDDaprStateStoreAzureStorage, outputresource.TypeARM, outputresource.KindDaprStateStoreAzureStorage, true, validation.ExpectedOutputResourceStatus{}),
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

			// Health has not yet been implemented
			SkipOutputResourceStatus: true,

			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameKV,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDKeyVault: validation.NewOutputResource(outputresource.LocalIDKeyVault, outputresource.TypeARM, outputresource.KindAzureKeyVault, true, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameDB,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureCosmosMongoAccount: validation.NewOutputResource(outputresource.LocalIDAzureCosmosMongoAccount, outputresource.TypeARM, outputresource.KindAzureCosmosAccountMongo, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDAzureCosmosDBMongo:      validation.NewOutputResource(outputresource.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, outputresource.KindAzureCosmosDBMongo, true, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameWebApp,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment:                    validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:                       validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentityKV: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentityKV, outputresource.TypeARM, outputresource.KindAzureUserAssignedManagedIdentity, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDRoleAssignmentKVKeys:          validation.NewOutputResource(outputresource.LocalIDRoleAssignmentKVKeys, outputresource.TypeARM, outputresource.KindAzureRoleAssignment, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDRoleAssignmentKVSecretsCerts:  validation.NewOutputResource(outputresource.LocalIDRoleAssignmentKVSecretsCerts, outputresource.TypeARM, outputresource.KindAzureRoleAssignment, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDAADPodIdentity:                validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, outputresource.KindAzurePodIdentity, true, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDKeyVaultSecret:                validation.NewOutputResource(outputresource.LocalIDKeyVaultSecret, outputresource.TypeARM, outputresource.KindAzureKeyVaultSecret, true, validation.ExpectedOutputResourceStatus{}),
						},
					},
				},
			},
		},
	})

	test.Test(t)
}
