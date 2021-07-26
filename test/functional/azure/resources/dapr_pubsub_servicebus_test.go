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
						ComponentName:   "nodesubscriber",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pythonpublisher",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pubsub",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDAzureServiceBusTopic: validation.NewOutputResource(workloads.LocalIDAzureServiceBusTopic, workloads.OutputResourceTypeArm, workloads.ResourceKindDaprPubSubTopicAzureServiceBus, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "nodesubscriber"),
						validation.NewK8sObjectForComponent(application, "pythonpublisher"),
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
						ComponentName:   "nodesubscriber",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pythonpublisher",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDDeployment: validation.NewOutputResource(workloads.LocalIDDeployment, workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "pubsub",
						OutputResources: map[string]validation.ExpectedOutputResource{
							workloads.LocalIDAzureServiceBusTopic: validation.NewOutputResource(workloads.LocalIDAzureServiceBusTopic, workloads.OutputResourceTypeArm, workloads.ResourceKindDaprPubSubTopicAzureServiceBus, false),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "nodesubscriber"),
						validation.NewK8sObjectForComponent(application, "pythonpublisher"),
					},
				},
			},
		},
	})

	test.Test(t)
}
