// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertToK8s(resource Resource, namespace string) (*unstructured.Unstructured, error) {
	annotations := map[string]string{}

	// Compute annotations to capture the name segments
	typeParts := strings.Split(resource.Type, "/")
	nameParts := strings.Split(resource.Name, "/")

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	spec := map[string]interface{}{}

	var applicationName string
	var resourceName string
	if len(nameParts) > 1 {
		applicationName = nameParts[1]
		annotations["radius.dev/application"] = applicationName
		spec = map[string]interface{}{
			"template":    runtime.RawExtension{Raw: data},
			"application": applicationName,
		}

		if len(nameParts) > 2 {
			resourceName = nameParts[2]
			annotations["radius.dev/resource"] = resourceName
			spec["resource"] = resourceName
		}
	}

	uns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "radius.dev/v1alpha3",
			"kind":       typeParts[len(typeParts)-1],
			"metadata": map[string]interface{}{
				"name":      nameParts[len(nameParts)-1],
				"namespace": namespace,
			},

			"spec": spec,
		},
	}

	uns.SetAnnotations(annotations)
	return uns, nil
}
