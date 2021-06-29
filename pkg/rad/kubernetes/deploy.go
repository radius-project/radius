// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/radius/pkg/rad/armtemplate"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

type KubernetesDeploymentClient struct {
	Client    dynamic.Interface
	Namespace string
}

func (c KubernetesDeploymentClient) Deploy(ctx context.Context, content string) error {
	template, err := armtemplate.Parse(content)
	if err != nil {
		return err
	}

	resources, err := armtemplate.Eval(template, armtemplate.TemplateOptions{})
	if err != nil {
		return err
	}

	for _, resource := range resources {
		gvr, kind, err := gvr(resource)
		if err != nil {
			return err
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
					"namespace": c.Namespace,
				},

				"spec": spec,
			},
		}

		uns.SetAnnotations(annotations)

		data, err := uns.MarshalJSON()
		if err != nil {
			return err
		}

		_, err = c.Client.Resource(gvr).Namespace(c.Namespace).Patch(ctx, name, types.ApplyPatchType, data, v1.PatchOptions{FieldManager: "rad"})
		if err != nil {
			return err
		}
	}

	return nil
}

func gvr(resource armtemplate.Resource) (schema.GroupVersionResource, string, error) {
	if resource.Type == radresources.ApplicationResourceType.Type() {
		return schema.GroupVersionResource{
			Group:    "applications.radius.dev",
			Version:  "v1alpha1",
			Resource: "applications",
		}, "Application", nil
	} else if resource.Type == radresources.ComponentResourceType.Type() {
		return schema.GroupVersionResource{
			Group:    "applications.radius.dev",
			Version:  "v1alpha1",
			Resource: "components",
		}, "Component", nil
	} else if resource.Type == radresources.DeploymentResourceType.Type() {
		return schema.GroupVersionResource{
			Group:    "applications.radius.dev",
			Version:  "v1alpha1",
			Resource: "deployments",
		}, "Deployment", nil
	}

	return schema.GroupVersionResource{}, "", fmt.Errorf("unsupported resource type '%s'", resource.Type)
}
