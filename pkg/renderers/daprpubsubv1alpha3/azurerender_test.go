// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

func Test_Render_Success(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     resourcekinds.DaprPubSubTopicAzureServiceBus,
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	result, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusTopic, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceType.Type)

	expected := map[string]string{
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

func Test_Render_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     resourcekinds.DaprPubSubTopicAzureServiceBus,
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a ServiceBus Topic", err.Error())
}
