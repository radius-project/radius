package handlers

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_ConstructDaprPubSubGeneric(t *testing.T) {
	metadata := map[string]interface{}{
		"foo": "bar",
	}
	metadataSerialized, err := json.Marshal(metadata)
	require.NoError(t, err, "Failed to serialize metadata")

	appName := "test-app"
	resourceName := "test-resource"
	pubsubType := "pubsub.kafka"
	daprPubSubVersion := "v1"
	daprVersion := "dapr.io/v1alpha1"
	k8sKind := "Component"
	properties := map[string]string{
		ResourceName:            resourceName,
		KubernetesNamespaceKey:  appName,
		KubernetesAPIVersionKey: daprVersion,
		KubernetesKindKey:       k8sKind,

		GenericDaprTypeKey:     pubsubType,
		GenericDaprVersionKey:  daprPubSubVersion,
		GenericDaprMetadataKey: string(metadataSerialized),
	}

	item, err := constructDaprGeneric(properties, appName, resourceName)
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
	assert.Equal(t, expected, item, "Resource spec does not match expected value")

}
