// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/mongodbv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_MongoDBManaged(t *testing.T) {
	t.Skip("Test takes too long on CI/CD to run, https://github.com/project-radius/radius/issues/1874")
	application := "azure-resources-mongodb-managed"
	template := "testdata/azure-resources-mongodb-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.DocumentDBDatabaseAccounts,
						Children: []validation.ExpectedChildResource{
							{
								Type: azresources.DocumentDBDatabaseAccountsMongodDBDatabases,
								Name: "starterdb",
							},
						},
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "todoapp",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "starterdb",
						ResourceType:    mongodbv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureCosmosAccount: validation.NewOutputResource(outputresource.LocalIDAzureCosmosAccount, outputresource.TypeARM, resourcekinds.AzureCosmosAccount, false, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureCosmosDBMongo: validation.NewOutputResource(outputresource.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, resourcekinds.AzureCosmosDBMongo, false, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_MongoDBUnmanaged(t *testing.T) {
	application := "azure-resources-mongodb-unmanaged"
	template := "testdata/azure-resources-mongodb-unmanaged.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.DocumentDBDatabaseAccounts,
						Tags: map[string]string{
							"radiustest": "azure-resources-mongodb-unmanaged",
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
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "todoapp",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "db",
						ResourceType:    mongodbv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureCosmosAccount: validation.NewOutputResource(outputresource.LocalIDAzureCosmosAccount, outputresource.TypeARM, resourcekinds.AzureCosmosAccount, false, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureCosmosDBMongo: validation.NewOutputResource(outputresource.LocalIDAzureCosmosDBMongo, outputresource.TypeARM, resourcekinds.AzureCosmosDBMongo, false, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_MongoDBUserSecrets(t *testing.T) {
	application := "azure-resources-mongodb-user-secrets"
	template := "testdata/azure-resources-mongodb-user-secrets.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "todoapp",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
