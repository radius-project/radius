package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	appName               = "test-app"
	resourceName          = "test-resource"
	daprVersion           = "dapr.io/v1alpha1"
	k8sKind               = "Component"
	stateStoreType        = "state.zookeeper"
	daprStateStoreVersion = "v1"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_ConstructDaprStateStoreGeneric(t *testing.T) {
	metadata := map[string]interface{}{
		"foo": "bar",
	}
	metadataSerialized, err := json.Marshal(metadata)
	require.NoError(t, err, "Failed to serialize metadata")

	properties := map[string]string{
		ManagedKey:              "false",
		ResourceName:            resourceName,
		KubernetesNamespaceKey:  appName,
		KubernetesAPIVersionKey: daprVersion,
		KubernetesKindKey:       k8sKind,

		GenericDaprStateStoreTypeKey:     stateStoreType,
		GenericDaprStateStoreVersionKey:  daprStateStoreVersion,
		GenericDaprStateStoreMetadataKey: string(metadataSerialized),
	}

	item, err := constructDaprStateStore(properties, appName, resourceName)
	require.NoError(t, err, "Unable to construct Dapr state store resource spec")

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
				"type":    stateStoreType,
				"version": daprStateStoreVersion,
				"metadata": []map[string]interface{}{
					{
						"name":  "foo",
						"value": "bar",
					},
				},
			},
		},
	}
	expectedJson, err := json.Marshal(expected)
	require.NoError(t, err)
	actualJson, err := json.Marshal(item)
	require.NoError(t, err)

	assert.Equal(t, string(expectedJson), string(actualJson), "Resource spec does not match expected value")

}
