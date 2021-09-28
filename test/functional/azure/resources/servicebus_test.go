// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/servicebusqueuev1alpha1"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_ServiceBusManaged(t *testing.T) {
	application := "azure-resources-servicebus-managed"
	template := "testdata/azure-resources-servicebus-managed.bicep"
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
								Type: azresources.ServiceBusNamespacesQueues,
								Name: "radius-queue1",
							},
						},
					},
				},
			},

			// ServiceBus deletion is currently flaky, tracked by: #768
			SkipAzureResources: true,

			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "sender",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "sbq",
						ResourceType:    servicebusqueuev1alpha1.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusQueue: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusQueue,
								outputresource.TypeARM,
								resourcekinds.AzureServiceBusQueue,
								true,
								true,
								rest.OutputResourceStatus{
									HealthState:       healthcontract.HealthStateHealthy,
									ProvisioningState: rest.Provisioned,
								}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "sender"),
					},
				},
			},
		},
	})

	test.Version = validation.AppModelV3
	test.SkipDeletion = true

	test.Test(t)
}
