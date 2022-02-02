// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
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
			"kind":    resourcekinds.DaprPubSubTopicAzureServiceBus,
			"managed": true,
			"topic":   "cool-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
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
			"kind":    resourcekinds.DaprPubSubTopicAzureServiceBus,
			"managed": true,
			// Topic is required
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
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
			"kind":     resourcekinds.DaprPubSubTopicAzureServiceBus,
			"managed":  false,
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
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
			"kind":     resourcekinds.DaprPubSubTopicAzureServiceBus,
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
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
			"kind":     resourcekinds.DaprPubSubTopicAzureServiceBus,
			"topic":    "not-allowed",
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/test-namespace/topics/test-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the 'topic' cannot be specified when 'managed' is not specified", err.Error())
}

func Test_Render_Any_Success(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":  resourcekinds.DaprPubSubTopicAny,
			"topic": "cool-topic",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
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

func Test_Render_Generic_Success(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind": resourcekinds.DaprPubSubTopicGeneric,
			"type": "pubsub.kafka",
			"metadata": map[string]interface{}{
				"foo": "bar",
			},
			"version": "v1",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	result, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprPubSubGeneric, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicGeneric, output.ResourceKind)
	require.False(t, output.Managed)

	metadata := map[string]interface{}{
		"foo": "bar",
	}
	metadataSerialized, err := json.Marshal(metadata)
	require.NoError(t, err, "Could not serialize metadata")

	expected := map[string]string{
		handlers.ManagedKey:              "false",
		handlers.ResourceName:            resource.ResourceName,
		handlers.KubernetesNamespaceKey:  resource.ApplicationName,
		handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:       "Component",

		handlers.GenericDaprTypeKey:     "pubsub.kafka",
		handlers.GenericDaprVersionKey:  "v1",
		handlers.GenericDaprMetadataKey: string(metadataSerialized),
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_Generic_MissingMetadata(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     resourcekinds.DaprPubSubTopicGeneric,
			"type":     "pubsub.kafka",
			"metadata": map[string]string{},
			"version":  "v1",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr component of type pubsub.kafka", err.Error())
}

func Test_Render_Generic_MissingType(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     resourcekinds.DaprPubSubTopicGeneric,
			"type":     "",
			"metadata": map[string]string{},
			"version":  "v1",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No type specified for generic Dapr component", err.Error())
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"kind":     resourcekinds.DaprPubSubTopicGeneric,
			"type":     "pubsub.kafka",
			"metadata": map[string]string{},
			"version":  "",
		},
	}

	renderer.PubSubs = SupportedAzurePubSubKindValues
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.Error())
}
