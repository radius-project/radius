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
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprpubsubv1alpha1"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_DaprPubSubServiceBusManaged(t *testing.T) {
	application := "azure-resources-dapr-pubsub-servicebus-managed"
	template := "testdata/azure-resources-dapr-pubsub-servicebus-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.ServiceBusNamespaces,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
						},
						Children: []validation.ExpectedChildResource{
							{
								Type: azresources.ServiceBusNamespacesTopics,
								Name: "TOPIC_A",
							},
						},
					},
				},
			},

			// This is currently flaky, tracked by #768
			SkipAzureResources: true,
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "publisher",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pubsub",
						ResourceType:    daprpubsubv1alpha1.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusTopic: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusTopic, outputresource.TypeARM, resourcekinds.DaprPubSubTopicAzureServiceBus, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "publisher"),
					},
				},
			},
		},
	})

	test.Version = validation.AppModelV3

	test.Test(t)
}

func Test_DaprPubSubServiceBusUnmanaged(t *testing.T) {
	application := "azure-resources-dapr-pubsub-servicebus-unmanaged"
	template := "testdata/azure-resources-dapr-pubsub-servicebus-unmanaged.bicep"
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
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "publisher",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pubsub",
						ResourceType:    daprpubsubv1alpha1.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusTopic: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusTopic, outputresource.TypeARM, resourcekinds.DaprPubSubTopicAzureServiceBus, false, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "publisher"),
					},
				},
			},
		},
	})

	test.Version = validation.AppModelV3

	test.Test(t)
}
