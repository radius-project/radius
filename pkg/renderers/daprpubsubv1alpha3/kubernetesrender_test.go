// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprpubsubv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	appName           = "test-app"
	resourceName      = "test-resource"
	pubsubType        = "pubsub.kafka"
	daprPubSubVersion = "v1"
	daprVersion       = "dapr.io/v1alpha1"
	k8sKind           = "Component"
)

func initRenderer() Renderer {
	return Renderer{
		PubSubs: SupportedKubernetesPubSubKindValues,
	}
}

func Test_Render_Generic_Kubernetes_Success(t *testing.T) {

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

	renderer := initRenderer()
	result, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 1)
	output := result.Resources[0]

	require.Equal(t, outputresource.LocalIDDaprPubSubGeneric, output.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, output.ResourceKind)
	require.False(t, output.Managed)

	expected := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]interface{}{
				"namespace": appName,
				"name":      resourceName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
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
	require.Equal(t, &expected, output.Resource)
}

func Test_Render_Generic_Kubernetes_MissingMetadata(t *testing.T) {
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

	renderer := initRenderer()
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No metadata specified for Dapr Pub/Sub component of type pubsub.kafka", err.Error())
}

func Test_Render_Generic_Kubernetes_MissingType(t *testing.T) {
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

	renderer := initRenderer()
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No type specified for generic Dapr Pub/Sub component", err.Error())
}

func Test_Render_Generic_Kubernetes_MissingVersion(t *testing.T) {
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

	renderer := initRenderer()
	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "No Dapr component version specified for generic Pub/Sub component", err.Error())
}

func Test_Kubernetes_ConstructDaprPubSubGeneric(t *testing.T) {
	properties := radclient.DaprPubSubTopicGenericResourceProperties{
		Type:    to.StringPtr("pubsub.kafka"),
		Version: to.StringPtr("v1"),
		Metadata: map[string]interface{}{
			"foo": "bar",
		},
	}

	item, err := constructPubSubResource(properties, appName, resourceName)
	require.NoError(t, err, "Unable to construct Pub/Sub resource spec")

	expected := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": daprVersion,
			"kind":       k8sKind,
			"metadata": map[string]interface{}{
				"namespace": appName,
				"name":      resourceName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
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
