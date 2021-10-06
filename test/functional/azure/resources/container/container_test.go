// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_ContainerHttpBinding(t *testing.T) {
	application := "azure-resources-container-httproute"
	template := "testdata/azure-resources-container-httproute.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// Intentionally Empty
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "frontend"),
						validation.NewK8sObjectForResource(application, "backend"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at azuretest.ApplicationTest) {
				// Verify ephemeral volume
				labelset := kubernetes.MakeSelectorLabels(application, "backend")

				matches, err := at.Options.K8sClient.CoreV1().Pods(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list pods")
				require.Lenf(t, matches.Items[0].Spec.Volumes, 1, "volumes should contain one item, instead it had: %+v", matches.Items[0].Spec.Volumes)
				volume := matches.Items[0].Spec.Volumes[0]
				require.NotNil(t, volume.EmptyDir, "volumes emptydir should have not been nil but it is")
				require.Equal(t, volume.EmptyDir.Medium, corev1.StorageMediumMemory, "volumes medium should be memory, instead it had: %v", volume.EmptyDir.Medium)
			},
		},
	})

	test.Test(t)
}

func Test_ContainerInboundRoute(t *testing.T) {
	application := "azure-resources-container-httproute-gateway"
	template := "testdata/azure-resources-container-httproute-gateway.bicep"
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
						validation.NewK8sObjectForResource(application, "frontend"),
						validation.NewK8sObjectForResource(application, "backend"),
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDIngress: validation.NewOutputResource(outputresource.LocalIDIngress, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
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
						validation.NewK8sObjectForResource(application, "frontend"),
						validation.NewK8sObjectForResource(application, "backend"),
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
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
