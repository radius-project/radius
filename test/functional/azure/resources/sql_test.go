// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/microsoftsqlv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_MicrosoftSQL_WithoutResourceID(t *testing.T) {
	application := "azure-resources-microsoft-sql"
	template := "testdata/azure-resources-microsoft-sql.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.SqlServers,
						Tags: map[string]string{
							"radiustest": "azure-resources-microsoft-sql",
						},
						Children: []validation.ExpectedChildResource{
							{
								Type:        azresources.SqlServersDatabases,
								Name:        "cool-database",
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
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, rest.ResourceType{Type: resourcekinds.Secret, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "db",
						ResourceType:    microsoftsqlv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureSqlServer:         validation.NewOutputResource(outputresource.LocalIDAzureSqlServer, rest.ResourceType{Type: resourcekinds.AzureSqlServer, Provider: providers.ProviderAzure}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureSqlServerDatabase: validation.NewOutputResource(outputresource.LocalIDAzureSqlServerDatabase, rest.ResourceType{Type: resourcekinds.AzureSqlServerDatabase, Provider: providers.ProviderAzure}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Objects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
