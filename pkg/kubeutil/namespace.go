// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubeutil

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/kubernetes"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchNamespace creates a namespace if it does not exist.
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
