// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

func Test_Render_Managed_Success_DefaultName(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			"topic":   "cool-topic",
		},
	}

	result, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusTopic, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceKind)
	require.True(t, output.Managed)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.ResourceName:            "test-resource",
		handlers.KubernetesNamespaceKey:  "test-app",
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",
		handlers.ServiceBusTopicNameKey:  "cool-topic",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_Managed_MissingTopic(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			// Topic is required
		},
	}

	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the 'topic' field is required when 'managed=true'", err.Error())
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed":  false,
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
		},
	}

	result, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusTopic, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceKind)
	require.False(t, output.Managed)

	expected := map[string]string{
		handlers.ManagedKey:                 "false",
		handlers.ResourceName:               "test-resource",
		handlers.KubernetesNamespaceKey:     "test-app",
		handlers.KubernetesAPIVersionKey:    "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:          "Component",
		handlers.ServiceBusNamespaceIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace",
		handlers.ServiceBusNamespaceNameKey: "test-namespace",
		handlers.ServiceBusTopicIDKey:       "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
		handlers.ServiceBusTopicNameKey:     "test-topic",
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
		},
	}

	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a ServiceBus Topic", err.Error())
}

func Test_Render_Unmanaged_SpecifiesTopicWithResource(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"topic":    "not-allowed",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
		},
	}

	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the 'topic' cannot be specified when 'managed' is not specified", err.Error())
}
