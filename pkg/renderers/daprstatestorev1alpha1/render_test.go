// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"kind":    "any",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreAzureStorage, resource.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, resource.ResourceKind)
	require.True(t, resource.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.KubernetesNameKey:       "test-component",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ResourceName:            "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"kind":     "any",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreAzureStorage, resource.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreAzureStorage, resource.ResourceKind)
	require.False(t, resource.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "false",
		handlers.KubernetesNameKey:       "test-component",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.StorageAccountIDKey:     "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
		handlers.StorageAccountNameKey:   "test-account",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"kind":     "any",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-storageAccounts/test-account",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a Storage Account", err.Error())
}

func Test_Render_Unmanaged_SpecifiesUmanagedWithoutResource(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"kind": "any",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_SQL_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"kind":    "state.sqlserver",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, outputresource.LocalIDDaprStateStoreSQLServer, resource.LocalID)
	require.Equal(t, resourcekinds.DaprStateStoreSQLServer, resource.ResourceKind)
	require.True(t, resource.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.KubernetesNameKey:       "test-component",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ResourceName:            "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_UnsupportedKind(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"kind":    "state.azure.cosmosdb",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("state.azure.cosmosdb is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedAzureStateStoreKindValues)), err.Error())
}

func Test_Render_SQL_Unmanaged_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedAzureStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"kind":     "state.sqlserver",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, "only Radius managed resources are supported for Dapr SQL Server", err.Error())
}

func Test_Render_K8s_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"kind":    "any",
			},
		},
		Namespace:     "default",
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 3)
	redisDeployment := resources[0]

	require.Equal(t, outputresource.LocalIDRedisDeployment, redisDeployment.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, redisDeployment.ResourceKind)
	resourceDeployment := redisDeployment.Resource.(*appsv1.Deployment)

	redisService := resources[1]
	require.Equal(t, outputresource.LocalIDRedisService, redisService.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, redisService.ResourceKind)
	resourceService := redisService.Resource.(*corev1.Service)

	dapr := resources[2]
	require.Equal(t, outputresource.LocalIDDaprStateStoreComponent, dapr.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, dapr.ResourceKind)
	resourceDapr := dapr.Resource.(*unstructured.Unstructured)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-component")
	matchLabels := kubernetes.MakeSelectorLabels("test-app", "test-component")

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-component", resourceDeployment.Name)
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
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(6379), port.ContainerPort)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, "test-component", resourceService.Name)
		require.Equal(t, labels, resourceService.Labels)
		require.Empty(t, resourceService.Annotations)

		spec := resourceService.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "redis", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(6379), port.Port)
		require.Equal(t, intstr.FromInt(6379), port.TargetPort)
	})

	t.Run("verify dapr", func(t *testing.T) {
		expected := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "dapr.io/v1alpha1",
				"kind":       "Component",
				"metadata": map[string]interface{}{
					"name":      "test-component",
					"namespace": "default",
					"labels":    kubernetes.MakeDescriptiveLabels("test-app", "test-component"),
				},
				"spec": map[string]interface{}{
					"type":    "state.redis",
					"version": "v1",
					"metadata": []interface{}{
						map[string]interface{}{
							"name":  "redisHost",
							"value": "test-component:6379",
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

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"kind":     "any",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, "only 'managed=true' is supported right now", err.Error())
}

func Test_Render_NonAny_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{SupportedKubernetesStateStoreKindValues}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"kind":     "state.sqlserver",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, fmt.Sprintf("state.sqlserver is not supported. Supported kind values: %s", getAlphabeticallySortedKeys(SupportedKubernetesStateStoreKindValues)), err.Error())
}
