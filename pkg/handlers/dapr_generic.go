// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"

	"github.com/project-radius/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	GenericDaprTypeKey     = "type"
	GenericDaprVersionKey  = "version"
	GenericDaprMetadataKey = "metadata"
)

func constructDaprGeneric(properties map[string]string, appName string, resourceName string) (unstructured.Unstructured, error) {
	// Convert the metadata to a map for easier access
	metadata := map[string]interface{}{}
	err := json.Unmarshal([]byte(properties[GenericDaprMetadataKey]), &metadata)
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/supported-state-stores/
	yamlListItems := []map[string]interface{}{}
	for k, v := range metadata {
		yamlItem := map[string]interface{}{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	// Translate into Dapr State Store schema
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      resourceName,
				"namespace": appName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":     properties[GenericDaprTypeKey],
				"version":  properties[GenericDaprVersionKey],
				"metadata": yamlListItems,
			},
		},
	}
	return item, nil
}

func getDaprGenericForDelete(ctx context.Context, options DeleteOptions) unstructured.Unstructured {
	properties := options.ExistingOutputResource.PersistedProperties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"name":      properties[KubernetesNameKey],
				"namespace": properties[KubernetesNamespaceKey],
			},
		},
	}

	return item
}
