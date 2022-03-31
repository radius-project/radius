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
	"github.com/project-radius/radius/pkg/renderers/daprpubsubv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprPubSubGeneric(t *testing.T) {
	application := "azure-resources-dapr-pubsub-generic"
	template := "testdata/azure-resources-dapr-pubsub-generic.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor:           azuretest.NewDeployStepExecutor(template),
			AzureResources:     &validation.AzureResourceSet{},
			SkipAzureResources: true,
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "publisher",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "pubsub",
						ResourceType:    daprpubsubv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDaprComponent: validation.NewOutputResource(outputresource.LocalIDDaprComponent, rest.ResourceType{Type: resourcekinds.DaprComponent, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Objects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "publisher"),
					},
				},
			},
		},
	})

	test.Test(t)
}
