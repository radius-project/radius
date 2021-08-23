// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armtemplate

import (
	"fmt"
	"strings"

	radresources "github.com/Azure/radius/pkg/radrp/resources"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type K8sInfo struct {
	Unstructured *unstructured.Unstructured
	GVR          schema.GroupVersionResource
	Name         string
}

func ConvertToK8s(resource Resource, namespace string) (K8sInfo, error) {
	gvr, kind, err := gvr(resource)
	if err != nil {
		return K8sInfo{}, err
	}

	// Calculate the resource name from the full resource name
	name := strings.ReplaceAll(resource.Name, "/", "-")

	annotations := map[string]string{}

	// Compute annotations to capture the name segments
	typeParts := strings.Split(resource.Type, "/")
	nameParts := strings.Split(resource.Name, "/")

	spec := map[string]interface{}{}

	k, ok := resource.Body["kind"]
	if ok {
		spec["kind"] = k
	}

	obj, ok := resource.Body["properties"]
	if ok {
		p, ok := obj.(map[string]interface{})
		if ok {
			for k, v := range p {
				spec[k] = v
			}
		}
	}

	// Temporarily add empty bindings if we have a component
	// as required by json schema
	// if kind == "Component" {
	// 	if spec["bindings"] == nil {
	// 		spec["bindings"] = map[string]interface{}{}
	// 	}
	// }

	hierarchy := []string{}
	for i, tp := range typeParts[1:] {
		annotations[fmt.Sprintf("radius.dev/%s", strings.ToLower(tp))] = strings.ToLower(nameParts[i])
		hierarchy = append(hierarchy, strings.ToLower(nameParts[i]))
	}
	spec["hierarchy"] = hierarchy

	uns := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": gvr.Group + "/" + gvr.Version,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},

			"spec": spec,
		},
	}

	uns.SetAnnotations(annotations)
	return K8sInfo{&uns, gvr, name}, nil
}

func gvr(resource Resource) (schema.GroupVersionResource, string, error) {
	if resource.Type == radresources.ApplicationResourceType.Type() {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "applications",
		}, "Application", nil
	} else if resource.Type == radresources.ComponentResourceType.Type() {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "components",
		}, "Component", nil
	} else if resource.Type == radresources.DeploymentResourceType.Type() {
		return schema.GroupVersionResource{
			Group:    "radius.dev",
			Version:  "v1alpha1",
			Resource: "deployments",
		}, "Deployment", nil
	}

	return schema.GroupVersionResource{}, "", fmt.Errorf("unsupported resource type '%s'", resource.Type)
}
