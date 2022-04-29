// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/daprstatestorev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprStateStoreTableStorage(t *testing.T) {
	application := "azure-resources-dapr-statestore-tablestorage"
	template := "testdata/azure-resources-dapr-statestore-tablestorage.bicep"

	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor:           azuretest.NewDeployStepExecutor(template, functional.GetMagpieImage()),
			AzureResources:     &validation.AzureResourceSet{},
			SkipAzureResources: true,
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "myapp",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, rest.ResourceType{Type: resourcekinds.Secret, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "mystore",
						ResourceType:    daprstatestorev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDaprComponent: validation.NewOutputResource(outputresource.LocalIDDaprComponent, rest.ResourceType{Type: resourcekinds.DaprStateStoreAzureStorage, Provider: providers.ProviderAzure}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Objects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
