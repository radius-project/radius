// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

import (
	"fmt"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DaprGeneric struct {
	Type     *string
	Version  *string
	Metadata map[string]any
}

func (daprGeneric DaprGeneric) Validate() error {
	if daprGeneric.Type == nil || *daprGeneric.Type == "" {
		return conv.NewClientErrInvalidRequest("No type specified for generic Dapr component")
	}

	if daprGeneric.Version == nil || *daprGeneric.Version == "" {
		return conv.NewClientErrInvalidRequest("No Dapr component version specified for generic Dapr component")
	}

	if daprGeneric.Metadata == nil {
		return conv.NewClientErrInvalidRequest(fmt.Sprintf("No metadata specified for Dapr component of type %s", *daprGeneric.Type))
	}

	return nil
}

func ConstructDaprGeneric(daprGeneric DaprGeneric, appName string, resourceName string, namespace string, resourceType string) (unstructured.Unstructured, error) {
	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/
	yamlListItems := []map[string]any{}
	for k, v := range daprGeneric.Metadata {
		yamlItem := map[string]any{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	// Translate into Dapr State Store schema
	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "dapr.io/v1alpha1",
			"kind":       "Component",
			"metadata": map[string]any{
				"namespace": namespace,
				"name":      kubernetes.NormalizeResourceName(resourceName),
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName, resourceType),
			},
			"spec": map[string]any{
				"type":     *daprGeneric.Type,
				"version":  *daprGeneric.Version,
				"metadata": yamlListItems,
			},
		},
	}
	return item, nil
}
