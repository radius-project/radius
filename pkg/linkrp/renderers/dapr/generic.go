/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dapr

import (
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type DaprGeneric struct {
	Type     *string
	Version  *string
	Metadata map[string]any
}

// Validate checks if the required fields of a DaprGeneric struct are set and returns an error if any of them are not.
func (daprGeneric DaprGeneric) Validate() error {
	if daprGeneric.Type == nil || *daprGeneric.Type == "" {
		return v1.NewClientErrInvalidRequest("No type specified for generic Dapr component")
	}

	if daprGeneric.Version == nil || *daprGeneric.Version == "" {
		return v1.NewClientErrInvalidRequest("No Dapr component version specified for generic Dapr component")
	}

	if daprGeneric.Metadata == nil {
		return v1.NewClientErrInvalidRequest(fmt.Sprintf("No metadata specified for Dapr component of type %s", *daprGeneric.Type))
	}

	return nil
}

// ConstructDaprGeneric constructs a Dapr component.
//
// The component name and resource name may be different. The component name is the name of the Dapr
// Kubernetes Component object to be created. Resource name is the name of the Radius resource.
func ConstructDaprGeneric(daprGeneric DaprGeneric, namespace string, componentName string, applicationName string, resourceName string, resourceType string) (unstructured.Unstructured, error) {
	// Convert the metadata map to a yaml list with keys name and value as per
	// Dapr specs: https://docs.dapr.io/reference/components-reference/
	yamlListItems := []any{} // K8s fake client requires this ..... :(
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
				"name":      kubernetes.NormalizeDaprResourceName(componentName),
				"labels":    kubernetes.MakeDescriptiveDaprLabels(applicationName, resourceName, resourceType),
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
