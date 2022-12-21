// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	applicationName      = "test-app"
	resourceID           = "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprPubSubBrokers/test-pub-sub"
	applicationID        = "/subscriptions/test-subscription/resourceGroups/test-rg/providers/Applications.Core/applications/test-app"
	environmentID        = "/subscriptions/test-subscription/resourceGroups/test-rg/providers/Applications.Core/environments/test-env"
	serviceBusResourceID = "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace"
	resourceName         = "test-pub-sub"
	pubsubType           = "pubsub.kafka"
	daprPubSubVersion    = "v1"
	daprVersion          = "dapr.io/v1alpha1"
	k8sKind              = "Component"
)

func Test_Render_Generic_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprComponent, output.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

	expected := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]interface{}{
				"namespace": "radius-test",
				"name":      kubernetes.NormalizeResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(applicationName, resourceName, ResourceType),
			},
			"spec": map[string]interface{}{
				"type":    pubsubType,
				"version": daprPubSubVersion,
				"metadata": []map[string]interface{}{
					{
						"name":  "foo",
						"value": "bar",
					},
				},
			},
		},
	}
	require.Equal(t, expected, output.Resource)
}

func Test_Render_Generic_MissingMetadata(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "No metadata specified for Dapr component of type pubsub.kafka", err.(*conv.ErrClientRP).Message)
}

func Test_Render_Generic_MissingType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
			Version: "v1",
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "No type specified for generic Dapr component", err.(*conv.ErrClientRP).Message)
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
			Type: "pubsub.kafka",
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.(*conv.ErrClientRP).Message)
}

func Test_ConstructDaprPubSubGeneric(t *testing.T) {
	properties := datamodel.DaprPubSubBrokerProperties{
		Type:    "pubsub.kafka",
		Version: "v1",
		Metadata: map[string]interface{}{
			"foo": "bar",
		},
	}
	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}
	item, err := dapr.ConstructDaprGeneric(daprGeneric, applicationName, resourceName, "radius-test", ResourceType)
	require.NoError(t, err, "Unable to construct Pub/Sub resource spec")

	expected := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]interface{}{
				"namespace": "radius-test",
				"name":      kubernetes.NormalizeResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(applicationName, resourceName, ResourceType),
			},
			"spec": map[string]interface{}{
				"type":    pubsubType,
				"version": daprPubSubVersion,
				"metadata": []map[string]interface{}{
					{
						"name":  "foo",
						"value": "bar",
					},
				},
			},
		},
	}
	actualYaml, err := yaml.Marshal(item)
	require.NoError(t, err, "Unable to convert resource spec to yaml")
	expectedYaml, _ := yaml.Marshal(expected)
	assert.Equal(t, string(expectedYaml), string(actualYaml), "Resource spec does not match expected value")
}

func Test_Render_DaprPubSubAzureServiceBus_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Topic:    "test-topic",
			Mode:     datamodel.LinkModeResource,
			Resource: serviceBusResourceID,
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusNamespace, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

	expected := map[string]string{
		handlers.ResourceName:               resourceName,
		handlers.KubernetesNamespaceKey:     "radius-test",
		handlers.ApplicationName:            applicationName,
		handlers.KubernetesAPIVersionKey:    "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:          "Component",
		handlers.ServiceBusNamespaceIDKey:   serviceBusResourceID,
		handlers.ServiceBusNamespaceNameKey: "test-namespace",
		handlers.ServiceBusTopicNameKey:     "test-topic",
	}
	require.Equal(t, expected, output.Resource)
	require.Equal(t, "test-topic", result.ComputedValues[TopicNameKey].Value)
}

func Test_Render_DaprPubSubMissingTopicName_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:     datamodel.LinkModeResource,
			Resource: serviceBusResourceID,
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDAzureServiceBusNamespace, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), result.ComputedValues[renderers.ComponentNameKey].Value)

	expected := map[string]string{
		handlers.ResourceName:               resourceName,
		handlers.KubernetesNamespaceKey:     "radius-test",
		handlers.ApplicationName:            applicationName,
		handlers.KubernetesAPIVersionKey:    "dapr.io/v1alpha1",
		handlers.KubernetesKindKey:          "Component",
		handlers.ServiceBusNamespaceIDKey:   serviceBusResourceID,
		handlers.ServiceBusNamespaceNameKey: "test-namespace",
		handlers.ServiceBusTopicNameKey:     resourceName,
	}
	require.Equal(t, expected, output.Resource)
	require.Equal(t, resourceName, result.ComputedValues[TopicNameKey].Value)
}

func Test_Render_DaprPubSubAzureServiceBus_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:     datamodel.LinkModeResource,
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.ServiceBus/namespaces/test-namespace/topics/test-topic",
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to a ServiceBus Namespace", err.(*conv.ErrClientRP).Message)
}

func Test_Render_UnsupportedMode(t *testing.T) {
	renderer := Renderer{SupportedPubSubModes}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:     "invalid",
			Resource: serviceBusResourceID,
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, fmt.Sprintf("invalid pub sub broker mode, Supported mode values: %s", getAlphabeticallySortedKeys(SupportedPubSubModes)), err.(*conv.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*conv.ErrClientRP).Message)
}

func Test_Render_EmptyApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: ResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
			Metadata: map[string]interface{}{
				"foo": "bar",
			},
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
}
