// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/Azure/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertToK8s(resource Resource, namespace string) (*unstructured.Unstructured, error) {
	annotations := map[string]string{}
	labels := map[string]string{}

	// Compute annotations to capture the name segments
	typeParts := strings.Split(resource.Type, "/")
	nameParts := strings.Split(resource.Name, "/")

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}
	applicationName, resourceName, resourceType := GetParts(nameParts, typeParts)

	if applicationName == "" {
		return nil, errors.New("application name is empty")
	}

	annotations[kubernetes.LabelRadiusApplication] = applicationName
	spec := map[string]interface{}{
		"template":    runtime.RawExtension{Raw: data},
		"application": applicationName,
	}

	if resourceType != "" && resourceName != "" {
		spec["resource"] = resourceName
		annotations[kubernetes.LabelRadiusResourceType] = resourceType
		annotations[kubernetes.LabelRadiusResource] = resourceName
	}

	labels = kubernetes.MakeResourceCRDLabels(applicationName, resourceType, resourceName)

	uns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "radius.dev/v1alpha3",
			"kind":       typeParts[len(typeParts)-1],
			"metadata": map[string]interface{}{
				"name":      nameParts[len(nameParts)-1],
				"namespace": namespace,
				"labels":    labels,
			},

			"spec": spec,
		},
	}

	uns.SetAnnotations(annotations)
	return uns, nil
}

func GetParts(nameParts, typeParts []string) (applicationName string, resourceName string, resourceType string) {
	if len(nameParts) > 1 {
		applicationName = nameParts[1]
		if len(nameParts) > 2 {
			resourceName = nameParts[2]
			resourceType = typeParts[len(typeParts)-1]
		}
	}
	return
}
