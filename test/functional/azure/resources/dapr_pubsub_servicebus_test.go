// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
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
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pubsub",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusTopic: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusTopic, outputresource.TypeARM, outputresource.KindDaprPubSubTopicAzureServiceBus, true, false, rest.OutputResourceStatus{}),
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
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pubsub",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusTopic: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusTopic, outputresource.TypeARM, outputresource.KindDaprPubSubTopicAzureServiceBus, false, false, rest.OutputResourceStatus{}),
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

	test.Test(t)
}
