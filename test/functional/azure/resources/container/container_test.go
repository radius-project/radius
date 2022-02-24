// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/renderers/gateway"
	"github.com/project-radius/radius/pkg/renderers/httproutev1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/azuretest"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_ContainerHttpBinding(t *testing.T) {
	application := "azure-resources-container-httproute"
	template := "testdata/azure-resources-container-httproute.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor:       azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
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
				labelset := kubernetes.MakeSelectorLabels(application, "backend")

				matches, err := at.Options.K8sClient.CoreV1().Pods(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list pods")

				found := false
				var volIndex int

				// Verify ephemeral volume
				for index, vol := range matches.Items[0].Spec.Volumes {
					if vol.Name == "my-volume" {
						found = true
						volIndex = index
					}
				}
				require.True(t, found, "volumes emptydir did not get mounted")
				volume := matches.Items[0].Spec.Volumes[volIndex]
				require.NotNil(t, volume.EmptyDir, "volumes emptydir should have not been nil but it is")
				require.Equal(t, volume.EmptyDir.Medium, corev1.StorageMediumMemory, "volumes medium should be memory, instead it had: %v", volume.EmptyDir.Medium)
			},
		},
	})

	test.Test(t)
}

func Test_ContainerGateway(t *testing.T) {
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
			Gateway: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "gateway"),
					},
				},
			},
			HttpRoute: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForResource(application, "frontend"),
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
							outputresource.LocalIDService:   validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDHttpRoute: validation.NewOutputResource(outputresource.LocalIDHttpRoute, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "gateway",
						ResourceType:    gateway.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDGateway: validation.NewOutputResource(outputresource.LocalIDGateway, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
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
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    httproutev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDService: validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerReadinessLiveness(t *testing.T) {
	application := "azure-resources-container-readiness-liveness"
	template := "testdata/azure-resources-container-readiness-liveness.bicep"
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
						validation.NewK8sObjectForResource(application, "backend"),
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
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
				require.Lenf(t, matches.Items, 1, "items should contain two match, instead it had: %+v", matches.Items)

				// Verify readiness probe
				require.Equal(t, "/healthz", matches.Items[0].Spec.Containers[0].ReadinessProbe.HTTPGet.Path)
				require.Equal(t, intstr.FromInt(3000), matches.Items[0].Spec.Containers[0].ReadinessProbe.HTTPGet.Port)
				require.Equal(t, int32(3), matches.Items[0].Spec.Containers[0].ReadinessProbe.InitialDelaySeconds)
				require.Equal(t, int32(4), matches.Items[0].Spec.Containers[0].ReadinessProbe.FailureThreshold)
				require.Equal(t, int32(20), matches.Items[0].Spec.Containers[0].ReadinessProbe.PeriodSeconds)
				require.Nil(t, matches.Items[0].Spec.Containers[0].ReadinessProbe.TCPSocket)
				require.Nil(t, matches.Items[0].Spec.Containers[0].ReadinessProbe.Exec)

				// Verify liveness probe
				require.Equal(t, []string{"ls", "/tmp"}, matches.Items[0].Spec.Containers[0].LivenessProbe.Exec.Command)
				require.Equal(t, int32(0), matches.Items[0].Spec.Containers[0].LivenessProbe.InitialDelaySeconds)
				require.Equal(t, int32(3), matches.Items[0].Spec.Containers[0].LivenessProbe.FailureThreshold)
				require.Equal(t, int32(10), matches.Items[0].Spec.Containers[0].LivenessProbe.PeriodSeconds)
				require.Nil(t, matches.Items[0].Spec.Containers[0].LivenessProbe.TCPSocket)
				require.Nil(t, matches.Items[0].Spec.Containers[0].LivenessProbe.HTTPGet)
			},
		},
	})

	test.Test(t)
}
