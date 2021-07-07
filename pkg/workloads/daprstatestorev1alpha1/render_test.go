// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
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
	renderer := Renderer{}

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

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprStateStoreAzureStorage, resource.ResourceKind)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.KubernetesNameKey:       "test-component",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ComponentNameKey:        "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprStateStoreAzureStorage, resource.ResourceKind)

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
	renderer := Renderer{}

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
	renderer := Renderer{}

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
	require.Equal(t, workloads.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_SQL_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprStateStoreSQLServer, resource.ResourceKind)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.KubernetesNameKey:       "test-component",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ComponentNameKey:        "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_UnsupportedKind(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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
	require.Equal(t, fmt.Sprintf("state.azure.cosmosdb is not supported. Supported kind values: %s", supportedStateStoreKindValues), err.Error())
}

func Test_Render_SQL_Unmanaged_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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
