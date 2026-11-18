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
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/yaml"
)

func deploymentStatusCRDs(t *testing.T) (*apiextensionsv1.CustomResourceDefinition, *apiextensionsv1.CustomResourceDefinition) {
	t.Helper()
	load := func(name string) *apiextensionsv1.CustomResourceDefinition {
		data, err := os.ReadFile("../../deploy/Chart/crds/radius/radapp.io_" + name + ".yaml")
		require.NoError(t, err)
		crd := &apiextensionsv1.CustomResourceDefinition{}
		require.NoError(t, yaml.Unmarshal(data, crd))
		return crd
	}
	return load("deploymentresources"), load("deploymenttemplates")
}

func TestDeploymentStatusSchemaUpgradePreservesSchemas(t *testing.T) {
	t.Parallel()
	resource, template := deploymentStatusCRDs(t)
	delete(resource.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties, "provisionedResourceID")
	template.Spec.Versions = append(template.Spec.Versions, *template.Spec.Versions[0].DeepCopy())
	template.Spec.Versions[1].Name = "v1alpha2"
	template.Spec.Versions[1].Storage = false
	status := template.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	coordinator := status.Properties["coordinator"]
	delete(coordinator.Properties, "message")
	delete(coordinator.Properties, "retryAfter")
	coordinator.Properties["futureField"] = apiextensionsv1.JSONSchemaProps{Type: "boolean"}
	status.Properties["coordinator"] = coordinator
	before := template.DeepCopy()
	patches := 0
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resource, template).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			patches++
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	require.NoError(t, ensureDeploymentResourceSchema(t.Context(), c))
	require.Equal(t, 2, patches)
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(template), template))
	expected := before.Spec.DeepCopy()
	expectedStatus := expected.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
	expectedCoordinator := expectedStatus.Properties["coordinator"]
	expectedCoordinator.Properties["message"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
	expectedCoordinator.Properties["retryAfter"] = apiextensionsv1.JSONSchemaProps{Type: "string", Format: "date-time"}
	require.Equal(t, *expected, template.Spec, "only the missing coordinator property may change")
	require.NoError(t, ensureDeploymentResourceSchema(t.Context(), c))
	require.Equal(t, 2, patches, "already migrated schemas must not be patched")
}

func TestDeploymentStatusSchemaUpgradeFailure(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"deploymentresources.radapp.io", "deploymenttemplates.radapp.io"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, failure := range []string{"get", "patch", "version", "schema", "status", "subresource", "type"} {
				t.Run(failure, func(t *testing.T) {
					t.Parallel()
					resource, template := deploymentStatusCRDs(t)
					target, field := resource, "provisionedResourceID"
					if name == template.Name {
						target, field = template, "coordinator"
					}
					status := target.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"]
					delete(status.Properties, field)
					switch failure {
					case "version":
						target.Spec.Versions[0].Name = "v1alpha2"
					case "schema":
						target.Spec.Versions[0].Schema = nil
					case "status":
						delete(target.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties, "status")
					case "subresource":
						target.Spec.Versions[0].Subresources.Status = nil
					case "type":
						status.Properties[field] = apiextensionsv1.JSONSchemaProps{Type: "integer"}
					}
					c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resource, template).WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
							if failure == "get" && key.Name == name {
								return errors.New("schema read denied")
							}
							return c.Get(ctx, key, obj, opts...)
						},
						Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
							if failure == "patch" && obj.GetName() == name {
								return errors.New("schema patch denied")
							}
							return c.Patch(ctx, obj, patch, opts...)
						},
					}).Build()
					err := ensureDeploymentResourceSchema(t.Context(), c)
					require.ErrorContains(t, err, name)
					if failure == "get" || failure == "patch" {
						require.ErrorContains(t, err, "denied")
					}
				})
			}
		})
	}
}

func TestDeploymentStatusSchemaUpgradeRetriesConflict(t *testing.T) {
	t.Parallel()
	resource, template := deploymentStatusCRDs(t)
	delete(template.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties, "coordinator")
	attempts := 0
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resource, template).WithInterceptorFuncs(interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			attempts++
			if attempts == 1 {
				return apierrors.NewConflict(schema.GroupResource{Group: apiextensionsv1.GroupName, Resource: "customresourcedefinitions"}, obj.GetName(), errors.New("concurrent update"))
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}).Build()
	require.NoError(t, ensureDeploymentResourceSchema(t.Context(), c))
	require.Equal(t, 2, attempts)
}

func TestDeploymentCoordinatorSchemaUpgradeRejectsInvalidNestedTypes(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"phase", "outputs", "items", "id", "state", "retryAfter", "retryAfterFormat"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			resource, template := deploymentStatusCRDs(t)
			coordinator := template.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties["coordinator"]
			switch field {
			case "phase", "outputs", "retryAfter":
				coordinator.Properties[field] = apiextensionsv1.JSONSchemaProps{Type: "integer"}
			case "retryAfterFormat":
				coordinator.Properties["retryAfter"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
			case "items":
				outputs := coordinator.Properties["outputs"]
				outputs.Items = &apiextensionsv1.JSONSchemaPropsOrArray{JSONSchemas: []apiextensionsv1.JSONSchemaProps{{Type: "string"}}}
				coordinator.Properties["outputs"] = outputs
			default:
				coordinator.Properties["outputs"].Items.Schema.Properties[field] = apiextensionsv1.JSONSchemaProps{Type: "integer"}
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resource, template).Build()
			require.ErrorContains(t, ensureDeploymentResourceSchema(t.Context(), c), "coordinator")
		})
	}
}

func TestDeploymentCoordinatorSchemaUpgradeRepairsPartialInventory(t *testing.T) {
	t.Parallel()
	resource, template := deploymentStatusCRDs(t)
	coordinator := template.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties["coordinator"]
	outputs := coordinator.Properties["outputs"]
	outputs.Items.Schema.Required = nil
	delete(outputs.Items.Schema.Properties, "state")
	coordinator.Properties["outputs"] = outputs
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resource, template).Build()
	require.NoError(t, ensureDeploymentResourceSchema(t.Context(), c))
	require.NoError(t, c.Get(t.Context(), client.ObjectKeyFromObject(template), template))
	_, expected := deploymentStatusCRDs(t)
	actualCoordinator := template.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties["coordinator"]
	require.Equal(t, "string", actualCoordinator.Properties["outputs"].Items.Schema.Properties["state"].Type)
	require.Equal(t, expected.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["status"].Properties["coordinator"].Properties["outputs"].Items.Schema.Required, actualCoordinator.Properties["outputs"].Items.Schema.Required)
}
