// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha3

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			"kind":    "any",
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreAzureStorage, output.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, output.ResourceKind)
	require.True(t, output.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.KubernetesNameKey:       "test-resource",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ResourceName:            "test-resource",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "any",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreAzureStorage, output.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, output.ResourceKind)
	require.False(t, output.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "false",
		handlers.KubernetesNameKey:       "test-resource",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.StorageAccountIDKey:     "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		handlers.StorageAccountNameKey:   "test-account",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "any",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a Storage Account", err.Error())
}

func Test_Render_Unmanaged_SpecifiesUmanagedWithoutResource(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": "any",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_SQL_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			"kind":    "state.sqlserver",
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreSQLServer, output.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreSQLServer, output.ResourceKind)
	require.True(t, output.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.KubernetesNameKey:       "test-resource",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ResourceName:            "test-resource",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_UnsupportedKind(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			"kind":    "state.azure.cosmosdb",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("state.azure.cosmosdb is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedAzureStateStoreKindValues)), err.Error())
}

func Test_Render_SQL_Unmanaged_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "state.sqlserver",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "only Radius managed resources are supported for Dapr SQL Server", err.Error())
}

func Test_Render_K8s_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			"kind":    "any",
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 3)
	redisDeployment := result.Resources[0]

	require.Equal(t, outputresource.LocalIDRedisDeployment, redisDeployment.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, redisDeployment.ResourceKind)
	resourceDeployment := redisDeployment.Resource.(*appsv1.Deployment)

	redisService := result.Resources[1]
	require.Equal(t, outputresource.LocalIDRedisService, redisService.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, redisService.ResourceKind)
	resourceService := redisService.Resource.(*corev1.Service)

	dapr := result.Resources[2]
	require.Equal(t, outputresource.LocalIDDaprStateStoreComponent, dapr.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, dapr.ResourceKind)
	resourceDapr := dapr.Resource.(*unstructured.Unstructured)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-resource")
	matchLabels := kubernetes.MakeSelectorLabels("test-app", "test-resource")

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-app-test-resource", resourceDeployment.Name)
		require.Equal(t, labels, resourceDeployment.Labels)
		require.Empty(t, resourceDeployment.Annotations)

		spec := resourceDeployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, "redis", container.Name)
		require.Equal(t, "redis", container.Image)
		require.Len(t, container.Ports, 1)

		port := container.Ports[0]
		require.Equal(t, corev1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(6379), port.ContainerPort)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, "test-app-test-resource", resourceService.Name)
		require.Equal(t, labels, resourceService.Labels)
		require.Empty(t, resourceService.Annotations)

		spec := resourceService.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "redis", port.Name)
		require.Equal(t, corev1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(6379), port.Port)
		require.Equal(t, intstr.FromInt(6379), port.TargetPort)
	})

	t.Run("verify dapr", func(t *testing.T) {
		expected := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dapr.io/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]interface{}{
					"name":   "test-resource",
					"labels": kubernetes.MakeDescriptiveLabels("test-app", "test-resource"),
				},
				"spec": map[string]interface{}{
					"type":    "state.redis",
					"version": "v1",
					"metadata": []interface{}{
						map[string]interface{}{
							"name":  "redisHost",
							"value": "test-app-test-resource:6379",
						},
						map[string]interface{}{
							"name":  "redisPassword",
							"value": "",
						},
					},
				},
			},
		}
		require.Equal(t, expected, resourceDapr)
	})
}

func Test_Render_K8s_Unmanaged_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "any",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "only 'managed=true' is supported right now", err.Error())
}

func Test_Render_NonAny_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesStateStoreKindValues}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     "state.sqlserver",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("state.sqlserver is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedKubernetesStateStoreKindValues)), err.Error())
}
