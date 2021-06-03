// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
)

func Test_Render_Managed_Success_DefaultName(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"topic":   "cool-topic",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(context.Background(), workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprPubSubTopicAzureServiceBus, resource.Type)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.ComponentNameKey:        "test-component",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ServiceBusTopicNameKey:  "cool-topic",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Managed_Success_SpecifyName(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"name":    "cool-servicebus",
				"topic":   "cool-topic",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(context.Background(), workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprPubSubTopicAzureServiceBus, resource.Type)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.ComponentNameKey:        "cool-servicebus",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ServiceBusTopicNameKey:  "cool-topic",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Managed_MissingTopic(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
				"name":    "cool-servicebus",
				// Topic is required
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "the 'topic' field is required when 'managed=true'", err.Error())
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"name":     "cool-servicebus",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(context.Background(), workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindDaprPubSubTopicAzureServiceBus, resource.Type)

	expected := map[string]string{
		handlers.ManagedKey:                 "false",
		handlers.ComponentNameKey:           "cool-servicebus",
		handlers.KubernetesNamespaceKey:     "test-app",
		handlers.KubernetesAPIVersionKey:    "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:          "Component",
		handlers.ServiceBusNamespaceIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace",
		handlers.ServiceBusNamespaceNameKey: "test-namespace",
		handlers.ServiceBusTopicIDKey:       "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
		handlers.ServiceBusTopicNameKey:     "test-topic",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a ServiceBus Topic", err.Error())
}

func Test_Render_Unmanaged_SpecifiesTopicWithResource(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"topic":    "not-allowed",
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "the 'topic' cannot be specified when 'managed' is not specified", err.Error())
}
