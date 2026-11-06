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

	radappiov1alpha3 "github.com/radius-project/radius/pkg/controller/api/radapp.io/v1alpha3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Helm does not upgrade CRDs in crds/. Migrate only the additive field this controller
// requires, without taking ownership of or replacing the rest of the installed schema.
func ensureDeploymentResourceSchema(ctx context.Context, c client.Client) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := c.Get(ctx, client.ObjectKey{Name: "deploymentresources.radapp.io"}, crd); err != nil {
			return err
		}
		base := crd.DeepCopy()
		for i := range crd.Spec.Versions {
			version := &crd.Spec.Versions[i]
			if version.Name != radappiov1alpha3.GroupVersion.Version {
				continue
			}
			if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
				return fmt.Errorf("DeploymentResource version %q has no structural schema", version.Name)
			}
			status, ok := version.Schema.OpenAPIV3Schema.Properties["status"]
			if !ok || status.Properties == nil || version.Subresources == nil || version.Subresources.Status == nil {
				return fmt.Errorf("DeploymentResource version %q has no protected status schema", version.Name)
			}
			if property, ok := status.Properties["provisionedResourceID"]; ok {
				if property.Type != "string" {
					return fmt.Errorf("DeploymentResource provisionedResourceID schema must be a string")
				}
				return nil
			}
			status.Properties["provisionedResourceID"] = apiextensionsv1.JSONSchemaProps{Type: "string"}
			version.Schema.OpenAPIV3Schema.Properties["status"] = status
			return c.Patch(ctx, crd, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}))
		}
		return fmt.Errorf("DeploymentResource CRD does not define version %q", radappiov1alpha3.GroupVersion.Version)
	})
}
