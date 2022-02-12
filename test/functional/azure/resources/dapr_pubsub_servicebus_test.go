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
	"github.com/project-radius/radius/pkg/renderers/daprpubsubv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprPubSubServiceBus(t *testing.T) {
	application := "azure-resources-dapr-pubsub-servicebus"
	template := "testdata/azure-resources-dapr-pubsub-servicebus.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.ServiceBusNamespaces,
						Tags: map[string]string{
							"radiustest": application,
						},
						UserManaged: true,
						Children: []validation.ExpectedChildResource{
							{
								Type:        azresources.ServiceBusNamespacesTopics,
								Name:        "TOPIC_A",
								UserManaged: true,
							},
						},
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "publisher",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "pubsub",
						ResourceType:    daprpubsubv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusTopic: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusTopic, outputresource.TypeARM, resourcekinds.DaprPubSubTopicAzureServiceBus, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "publisher"),
					},
				},
			},
		},
	})

	test.Test(t)
}
