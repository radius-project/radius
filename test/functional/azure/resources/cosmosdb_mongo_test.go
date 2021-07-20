// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/workloads"
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
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "db",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDAzureCosmosDBMongo: validation.NewOutputResource(workloads.LocalIDAzureCosmosDBMongo, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureCosmosDBMongo, true),
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
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "db",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDAzureCosmosDBMongo, workloads.OutputResourceTypeArm, workloads.ResourceKindAzureCosmosDBMongo, false),
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
