/*
Copyright 2026 The Radius Authors.

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

package controller

import (
	"context"
	"fmt"
	"slices"

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helm does not upgrade CRDs in crds/. Migrate only the additive fields this controller
// requires, without taking ownership of or replacing the rest of the installed schema.
func ensureDeploymentResourceSchema(ctx context.Context, c client.Client) error {
	if err := ensureDeploymentStatusField(ctx, c, "deploymentresources.radapp.io", "provisionedResourceID", apiextensionsv1.JSONSchemaProps{Type: "string"}); err != nil {
		return err
	}
	return ensureDeploymentStatusField(ctx, c, "deploymenttemplates.radapp.io", "coordinator", apiextensionsv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextensionsv1.JSONSchemaProps{
			"phase":         {Type: "string"},
			"resourceID":    {Type: "string"},
			"submittedHash": {Type: "string"},
			"message":       {Type: "string"},
			"retryAfter":    {Type: "string", Format: "date-time"},
			"outputs": {
				Type: "array",
				Items: &apiextensionsv1.JSONSchemaPropsOrArray{Schema: &apiextensionsv1.JSONSchemaProps{
					Type: "object",
					Properties: map[string]apiextensionsv1.JSONSchemaProps{
						"id":    {Type: "string"},
						"state": {Type: "string"},
					},
					Required: []string{"id", "state"},
				}},
			},
		},
	})
}

func ensureDeploymentStatusField(ctx context.Context, c client.Client, name, field string, expected apiextensionsv1.JSONSchemaProps) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := c.Get(ctx, client.ObjectKey{Name: name}, crd); err != nil {
			return fmt.Errorf("get %s schema: %w", name, err)
		}
		base := crd.DeepCopy()
		for i := range crd.Spec.Versions {
			version := &crd.Spec.Versions[i]
			if version.Name != radappiov1alpha3.GroupVersion.Version {
				continue
			}
			if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
				return fmt.Errorf("%s version %q has no structural schema", name, version.Name)
			}
			status, ok := version.Schema.OpenAPIV3Schema.Properties["status"]
			if !ok || status.Properties == nil || version.Subresources == nil || version.Subresources.Status == nil {
				return fmt.Errorf("%s version %q has no protected status schema", name, version.Name)
			}
			property, exists := status.Properties[field]
			changed := !exists
			if exists {
				var err error
				changed, err = mergeDeploymentStatusSchema(&property, expected, name+".status."+field)
				if err != nil {
					return err
				}
			} else {
				property = *expected.DeepCopy()
			}
			if !changed {
				return nil
			}
			status.Properties[field] = property
			version.Schema.OpenAPIV3Schema.Properties["status"] = status
			if err := c.Patch(ctx, crd, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); err != nil {
				return fmt.Errorf("patch %s schema: %w", name, err)
			}
			return nil
		}
		return fmt.Errorf("%s CRD does not define version %q", name, radappiov1alpha3.GroupVersion.Version)
	})
}

func mergeDeploymentStatusSchema(actual *apiextensionsv1.JSONSchemaProps, expected apiextensionsv1.JSONSchemaProps, path string) (bool, error) {
	if actual.Type != expected.Type {
		return false, fmt.Errorf("%s schema must be a %s", path, expected.Type)
	}
	if expected.Format != "" && actual.Format != expected.Format {
		return false, fmt.Errorf("%s schema must use format %s", path, expected.Format)
	}
	changed := false
	for name, expectedProperty := range expected.Properties {
		property, exists := actual.Properties[name]
		if !exists {
			if actual.Properties == nil {
				actual.Properties = map[string]apiextensionsv1.JSONSchemaProps{}
			}
			actual.Properties[name] = *expectedProperty.DeepCopy()
			changed = true
			continue
		}
		updated, err := mergeDeploymentStatusSchema(&property, expectedProperty, path+"."+name)
		if err != nil {
			return false, err
		}
		actual.Properties[name] = property
		changed = changed || updated
	}
	if expected.Items != nil {
		if actual.Items == nil {
			actual.Items = expected.Items.DeepCopy()
			changed = true
		} else if actual.Items.Schema == nil {
			return false, fmt.Errorf("%s schema must define array items", path)
		} else {
			updated, err := mergeDeploymentStatusSchema(actual.Items.Schema, *expected.Items.Schema, path+"[]")
			if err != nil {
				return false, err
			}
			changed = changed || updated
		}
	}
	for _, required := range expected.Required {
		if !slices.Contains(actual.Required, required) {
			actual.Required = append(actual.Required, required)
			changed = true
		}
	}
	return changed, nil
}
