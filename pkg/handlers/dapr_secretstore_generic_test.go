// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"encoding/json"
	"testing"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	secretStoreType        = "secretstores.azure.keyvault"
	daprSecretStoreVersion = "v1"
)

func Test_ConstructDaprSecretStoreGeneric(t *testing.T) {
	metadata := map[string]interface{}{
		"foo": "bar",
	}
	metadataSerialized, err := json.Marshal(metadata)
	require.NoError(t, err, "Failed to serialize metadata")

	properties := map[string]string{
		ResourceName:            resourceName,
		KubernetesNamespaceKey:  appName,
		KubernetesAPIVersionKey: daprVersion,
		KubernetesKindKey:       k8sKind,

		GenericDaprTypeKey:     secretStoreType,
		GenericDaprVersionKey:  daprSecretStoreVersion,
		GenericDaprMetadataKey: string(metadataSerialized),
	}

	item, err := constructDaprGeneric(properties, appName, resourceName)
	require.NoError(t, err, "Unable to construct Dapr secret store resource spec")

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
				"type":    secretStoreType,
				"version": daprSecretStoreVersion,
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
