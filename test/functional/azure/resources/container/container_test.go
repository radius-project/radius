// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers/azurefilesharev1alpha3"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/gateway"
	"github.com/Azure/radius/pkg/renderers/httproutev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
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
			Executor: azuretest.NewDeployStepExecutor(template),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					{
						Type: azresources.StorageStorageAccounts,
						Tags: map[string]string{
							keys.TagRadiusApplication: application,
							keys.TagRadiusResource:    "myshare",
						},
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
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "myshare",
						ResourceType:    azurefilesharev1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDAzureFileShare:               validation.NewOutputResource(outputresource.LocalIDAzureFileShare, outputresource.TypeARM, resourcekinds.AzureFileShare, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDAzureFileShareStorageAccount: validation.NewOutputResource(outputresource.LocalIDAzureFileShareStorageAccount, outputresource.TypeARM, resourcekinds.AzureFileShareStorageAccount, true, false, rest.OutputResourceStatus{}),
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

				// Verify persistent volume
				found = false
				for index, vol := range matches.Items[0].Spec.Volumes {
					if vol.Name == "my-volume2" {
						found = true
						volIndex = index
					}
				}
				require.True(t, found, "persistent volume did not get mounted")
				volume = matches.Items[0].Spec.Volumes[volIndex]
				require.NotNil(t, volume.AzureFile, "volumes azure file should have not been nil but it is")
				require.Equal(t, volume.AzureFile.ShareName, "myshare")
				require.Equal(t, volume.AzureFile.SecretName, "backend")
			},
		},
	})

	test.Test(t)
}

func Test_ContainerGateway(t *testing.T) {
	t.Skip("Skipping to merge azure gateway support which requires infra changes.")
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
							outputresource.LocalIDService:   validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDHttpRoute: validation.NewOutputResource(outputresource.LocalIDHttpRoute, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "gateway",
						ResourceType:    gateway.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDGateway: validation.NewOutputResource(outputresource.LocalIDGateway, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
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

				// Verify readiness probe
				require.Equal(t, "/healthz", matches.Items[0].Spec.Containers[0].ReadinessProbe.HTTPGet.Path)
				require.Equal(t, intstr.FromInt(8080), matches.Items[0].Spec.Containers[0].ReadinessProbe.HTTPGet.Port)
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
