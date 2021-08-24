// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
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

			SkipOutputResourceStatus: false,

			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "sender",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(
								outputresource.LocalIDDeployment,
								outputresource.TypeKubernetes,
								outputresource.KindKubernetes,
								true,
								validation.ExpectedOutputResourceStatus{
									HealthState:              "",
									HealthStateErrorDetails:  "",
									ProvisioningState:        rest.Provisioned,
									ProvisioningErrorDetails: "",
								}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "sbq",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureServiceBusQueue: validation.NewOutputResource(outputresource.LocalIDAzureServiceBusQueue,
								outputresource.TypeARM,
								outputresource.KindAzureServiceBusQueue,
								true,
								validation.ExpectedOutputResourceStatus{
									HealthState:              healthcontract.HealthStateHealthy,
									HealthStateErrorDetails:  "",
									ProvisioningState:        rest.Provisioned,
									ProvisioningErrorDetails: "",
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

	test.Test(t)
}
