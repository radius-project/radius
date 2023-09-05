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

package kubeutil

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchNamespace creates a new namespace with the given name and labels it with the given label, then applies it to the
// cluster. It returns an error if the patch operation fails.
func PatchNamespace(ctx context.Context, client runtime_client.Client, namespace string) error {
	ns := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": namespace,
				"labels": map[string]any{
					kubernetes.LabelManagedBy: kubernetes.LabelManagedByRadiusRP,
				},
			},
		},
	}

	err := client.Patch(ctx, ns, runtime_client.Apply, &runtime_client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return fmt.Errorf("error applying namespace: %w", err)
	}

	return nil
}
