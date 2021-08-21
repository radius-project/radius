// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package servicebusqueuev1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
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
				"queue":   "cool-queue",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusQueue, resource.LocalID)
	require.Equal(t, outputresource.KindAzureServiceBusQueue, resource.Kind)
	require.Equal(t, outputresource.TypeARM, resource.Type)
	require.True(t, resource.Managed)

	expected := map[string]string{
		handlers.ManagedKey:             "true",
		handlers.ServiceBusQueueNameKey: "cool-queue",
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
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/queues/test-queue",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusQueue, resource.LocalID)
	require.Equal(t, outputresource.KindAzureServiceBusQueue, resource.Kind)
	require.Equal(t, outputresource.TypeARM, resource.Type)
	require.False(t, resource.Managed)

	expected := map[string]string{
		handlers.ManagedKey:                 "false",
		handlers.ServiceBusNamespaceIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace",
		handlers.ServiceBusNamespaceNameKey: "test-namespace",
		handlers.ServiceBusQueueIDKey:       "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/queues/test-queue",
		handlers.ServiceBusQueueNameKey:     "test-queue",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": false,
				// Resource is required
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, workloads.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
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
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/queues/test-queue",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a ServiceBus Queue", err.Error())
}
