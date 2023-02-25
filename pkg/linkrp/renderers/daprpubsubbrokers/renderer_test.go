// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubbrokers

import (
	"context"
	"fmt"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/dapr"
	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
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
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	result, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, rpv1.LocalIDDaprComponent, output.LocalID)
	require.Equal(t, resourcekinds.DaprComponent, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), result.ComputedValues[linkrp.ComponentNameKey].Value)

	expected := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]any{
				"namespace": "radius-test",
				"name":      kubernetes.NormalizeResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(applicationName, resourceName, linkrp.DaprPubSubBrokersResourceType),
			},
			"spec": map[string]any{
				"type":    pubsubType,
				"version": daprPubSubVersion,
				"metadata": []map[string]any{
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
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No metadata specified for Dapr component of type pubsub.kafka", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_MissingType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Metadata: map[string]any{
				"foo": "bar",
			},
			Version: "v1",
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No type specified for generic Dapr component", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Generic_MissingVersion(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: environmentID,
			},
			Mode: datamodel.LinkModeValues,
			Metadata: map[string]any{
				"foo": "bar",
			},
			Type: "pubsub.kafka",
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "No Dapr component version specified for generic Dapr component", err.(*v1.ErrClientRP).Message)
}

func Test_ConstructDaprPubSubGeneric(t *testing.T) {
	properties := datamodel.DaprPubSubBrokerProperties{
		Type:    "pubsub.kafka",
		Version: "v1",
		Metadata: map[string]any{
			"foo": "bar",
		},
	}
	daprGeneric := dapr.DaprGeneric{
		Type:     &properties.Type,
		Version:  &properties.Version,
		Metadata: properties.Metadata,
	}
	item, err := dapr.ConstructDaprGeneric(daprGeneric, applicationName, resourceName, "radius-test", linkrp.DaprPubSubBrokersResourceType)
	require.NoError(t, err, "Unable to construct Pub/Sub resource spec")

	expected := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]any{
				"namespace": "radius-test",
				"name":      kubernetes.NormalizeResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(applicationName, resourceName, linkrp.DaprPubSubBrokersResourceType),
			},
			"spec": map[string]any{
				"type":    pubsubType,
				"version": daprPubSubVersion,
				"metadata": []map[string]any{
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
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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

	require.Equal(t, rpv1.LocalIDAzureServiceBusNamespace, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), result.ComputedValues[linkrp.ComponentNameKey].Value)

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
	require.Equal(t, "test-topic", result.ComputedValues[linkrp.TopicNameKey].Value)
}

func Test_Render_DaprPubSubMissingTopicName_Success(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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

	require.Equal(t, rpv1.LocalIDAzureServiceBusNamespace, output.LocalID)
	require.Equal(t, resourcekinds.DaprPubSubTopicAzureServiceBus, output.ResourceType.Type)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), result.ComputedValues[linkrp.ComponentNameKey].Value)

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
	require.Equal(t, resourceName, result.ComputedValues[linkrp.TopicNameKey].Value)
}

func Test_Render_DaprPubSubAzureServiceBus_InvalidResourceType(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to a ServiceBus Namespace", err.(*v1.ErrClientRP).Message)
}

func Test_Render_UnsupportedMode(t *testing.T) {
	renderer := Renderer{SupportedPubSubModes}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, fmt.Sprintf("invalid pub sub broker mode, Supported mode values: %s", getAlphabeticallySortedKeys(SupportedPubSubModes)), err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_EmptyApplicationID(t *testing.T) {
	renderer := Renderer{}
	resource := datamodel.DaprPubSubBroker{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   resourceID,
				Name: resourceName,
				Type: linkrp.DaprPubSubBrokersResourceType,
			},
		},
		Properties: datamodel.DaprPubSubBrokerProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: environmentID,
			},
			Mode:    datamodel.LinkModeValues,
			Type:    "pubsub.kafka",
			Version: "v1",
			Metadata: map[string]any{
				"foo": "bar",
			},
		},
	}
	renderer.PubSubs = SupportedPubSubModes
	_, err := renderer.Render(context.Background(), &resource, renderers.RenderOptions{Namespace: "radius-test"})
	require.NoError(t, err)
}
