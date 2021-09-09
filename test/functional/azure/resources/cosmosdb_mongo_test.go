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
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_CosmosDBMongoManaged(t *testing.T) {
	application := "azure-resources-cosmosdb-mongo-managed"
	template := "testdata/azure-resources-cosmosdb-mongo-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.DocumentDBDatabaseAccounts,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
							keys.TagRadiusComponent:   "db",
						},
						Children: []validation.ExpectedChildResource{
							{
								Type: azresources.DocumentDBDatabaseAccountsMongodDBDatabases,
								Name: "db",
							},
						},
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "todoapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "db",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureCosmosMongoAccount: validation.NewOutputResource(outputresource.LocalIDAzureCosmosMongoAccount, outputresource.TypeARM, resourcekinds.AzureCosmosAccountMongo, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureCosmosDBMongo:      validation.NewOutputResource(outputresource.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, resourcekinds.AzureCosmosDBMongo, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_CosmosDBMongoUnmanaged(t *testing.T) {
	application := "azure-resources-cosmosdb-mongo-unmanaged"
	template := "testdata/azure-resources-cosmosdb-mongo-unmanaged.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.DocumentDBDatabaseAccounts,
						Tags: map[string]string{
							"radiustest": "azure-resources-cosmosdb-mongo-unmanaged",
						},
						Children: []validation.ExpectedChildResource{
							{
								Type:        azresources.DocumentDBDatabaseAccountsMongodDBDatabases,
								Name:        "mydb",
								UserManaged: true,
							},
						},
						UserManaged: true,
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "todoapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "db",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureCosmosMongoAccount: validation.NewOutputResource(outputresource.LocalIDAzureCosmosMongoAccount, outputresource.TypeARM, resourcekinds.AzureCosmosAccountMongo, false, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureCosmosDBMongo:      validation.NewOutputResource(outputresource.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, resourcekinds.AzureCosmosDBMongo, false, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
