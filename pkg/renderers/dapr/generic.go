// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

import (
	"errors"
	"fmt"

	"github.com/project-radius/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DaprGeneric struct {
	Type     *string
	Version  *string
	Metadata map[string]interface{}
}

func ValidateDaprGenericObject(daprGeneric DaprGeneric) error {
	if daprGeneric.Type == nil || *daprGeneric.Type == "" {
		return errors.New("No type specified for generic Dapr component")
	}

	if daprGeneric.Version == nil || *daprGeneric.Version == "" {
		return errors.New("No Dapr component version specified for generic Dapr component")
	}

	if daprGeneric.Metadata == nil || len(daprGeneric.Metadata) == 0 {
		return fmt.Errorf("No metadata specified for Dapr component of type %s", *daprGeneric.Type)
	}

	return nil
}

func ConstructDaprGeneric(daprGeneric DaprGeneric, appName string, resourceName string) (unstructured.Unstructured, error) {
	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/
	yamlListItems := []map[string]interface{}{}
	for k, v := range daprGeneric.Metadata {
		yamlItem := map[string]interface{}{
			"name":  k,
			"value": v,
		}
		yamlListItems = append(yamlListItems, yamlItem)
	}

	// Translate into Dapr State Store schema
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dapr.io/v1alpha1",
			"kind":       "Component",
			"metadata": map[string]interface{}{
				"namespace": appName,
				"name":      resourceName,
				"labels":    kubernetes.MakeDescriptiveLabels(appName, resourceName),
			},
			"spec": map[string]interface{}{
				"type":     *daprGeneric.Type,
				"version":  *daprGeneric.Version,
				"metadata": yamlListItems,
			},
		},
	}
	return item, nil
}
