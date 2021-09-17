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
	// Calculate the resource name from the full resource name
	// name := strings.ReplaceAll(resource.Name, "/", "-")

	annotations := map[string]string{}

	// Compute annotations to capture the name segments
	typeParts := strings.Split(resource.Type, "/")
	nameParts := strings.Split(resource.Name, "/")

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}

	if len(nameParts) > 1 {
		annotations["radius.dev/application"] = nameParts[1]
		if len(nameParts) > 2 {
			annotations["radius.dev/resource"] = nameParts[2]
		}
	}

	// for i, tp := range typeParts[1:] {
	// 	annotations[fmt.Sprintf("radius.dev/%s", strings.ToLower(tp))] = strings.ToLower(nameParts[i])
	// }

	uns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "radius.dev/v1alpha3",
			"kind":       typeParts[len(typeParts)-1],
			"metadata": map[string]interface{}{
				"name":      nameParts[len(nameParts)-1],
				"namespace": namespace,
			},

			"spec": map[string]interface{}{
				"template": runtime.RawExtension{Raw: data},
			},
		},
	}

	uns.SetAnnotations(annotations)
	return uns, nil
}
