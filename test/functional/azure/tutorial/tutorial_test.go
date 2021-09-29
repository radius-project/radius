// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tutorial_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_TutorialDaprMicroservices(t *testing.T) {
	application := "dapr-hello"
	template := "../../../../docs/content/user-guides/tutorial/dapr-microservices/dapr-microservices.bicep"
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
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pythonapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "statestore",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDaprStateStoreAzureStorage: validation.NewOutputResource(outputresource.LocalIDDaprStateStoreAzureStorage, outputresource.TypeARM, resourcekinds.DaprStateStoreAzureStorage, true, false, rest.OutputResourceStatus{}),
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
	template := "../../../../docs/content/user-guides/tutorial/webapp/code/template.bicep"
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
							outputresource.LocalIDKeyVault: validation.NewOutputResource(outputresource.LocalIDKeyVault, outputresource.TypeARM, resourcekinds.AzureKeyVault, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameDB,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureCosmosMongoAccount: validation.NewOutputResource(outputresource.LocalIDAzureCosmosMongoAccount, outputresource.TypeARM, resourcekinds.AzureCosmosAccountMongo, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureCosmosDBMongo:      validation.NewOutputResource(outputresource.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, resourcekinds.AzureCosmosDBMongo, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: applicationName,
						ComponentName:   componentNameWebApp,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment:                    validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:                       validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:                        validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDUserAssignedManagedIdentityKV: validation.NewOutputResource(outputresource.LocalIDUserAssignedManagedIdentityKV, outputresource.TypeARM, resourcekinds.AzureUserAssignedManagedIdentity, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDRoleAssignmentKVKeys:          validation.NewOutputResource(outputresource.LocalIDRoleAssignmentKVKeys, outputresource.TypeARM, resourcekinds.AzureRoleAssignment, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDRoleAssignmentKVSecretsCerts:  validation.NewOutputResource(outputresource.LocalIDRoleAssignmentKVSecretsCerts, outputresource.TypeARM, resourcekinds.AzureRoleAssignment, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAADPodIdentity:                validation.NewOutputResource(outputresource.LocalIDAADPodIdentity, outputresource.TypeAADPodIdentity, resourcekinds.AzurePodIdentity, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDKeyVaultSecret:                validation.NewOutputResource(outputresource.LocalIDKeyVaultSecret, outputresource.TypeARM, resourcekinds.AzureKeyVaultSecret, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
		},
	})

	test.Test(t)
}
