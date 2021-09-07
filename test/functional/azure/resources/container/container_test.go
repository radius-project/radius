// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_ContainerHttpBinding(t *testing.T) {
	application := "azure-resources-container-httpbinding"
	template := "testdata/azure-resources-container-httpbinding.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// Intentionally Empty
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "frontend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				appclient := radclient.NewApplicationClient(at.Options.ARMConnection, at.Options.Environment.SubscriptionID)

				// get application and verify name
				response, err := appclient.Get(ctx, at.Options.Environment.ResourceGroup, application, nil)
				require.NoError(t, err)
				assert.Equal(t, application, *response.ApplicationResource.Name)
			},
		},
	})

	test.Test(t)
}

func Test_ContainerInboundRoute(t *testing.T) {
	application := "azure-resources-container-inboundroute"
	template := "testdata/azure-resources-container-inboundroute.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// Intentionally Empty
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "frontend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDIngress:    validation.NewOutputResource(outputresource.LocalIDIngress, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				// Verify that we've created an ingress resource. We don't verify reachability because allocating
				// a public IP can take a few minutes.
				labelset := kubernetes.MakeSelectorLabels(application, "frontend")
				matches, err := at.Options.K8sClient.NetworkingV1().Ingresses(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list ingresses")
				require.Lenf(t, matches.Items, 1, "items should contain one match, instead it had: %+v", matches.Items)
			},
		},
	})

	test.Test(t)
}

func Test_ContainerManualScale(t *testing.T) {
	application := "azure-resources-container-manualscale"
	template := "testdata/azure-resources-container-manualscale.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// Intentionally Empty
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "frontend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				// Verify there are two pods created for backend.
				labelset := kubernetes.MakeSelectorLabels(application, "backend")

				matches, err := at.Options.K8sClient.CoreV1().Pods(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list pods")
				require.Lenf(t, matches.Items, 2, "items should contain two match, instead it had: %+v", matches.Items)
			},
		},
	})

	test.Test(t)
}
