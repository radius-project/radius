// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
)

func Test_Render_Managed_Success(t *testing.T) {
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

	resources, err := renderer.Render(context.Background(), workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprStateStoreAzureStorage, resource.Type)

	expected := map[string]string{
		handlers.ManagedKey:                "true",
		handlers.KubernetesNameKey:         "test-component",
		handlers.KubernetesNamespaceKey:    "test-app",
		handlers.KubernetesAPIVersionKey:   "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:         "Component",
		handlers.StorageAccountBaseNameKey: "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}
