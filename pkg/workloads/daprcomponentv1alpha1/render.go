// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprcomponentv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/workloads"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Renderer is the WorkloadRenderer implementation for the dapr component workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for dapr component workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	// dapr doesn't support any services
	return nil, fmt.Errorf("the service is not supported")
}

// Render is the WorkloadRenderer implementation for dapr component workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	resource := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "dapr.io/v1",
			"kind":       "Component",
			"metadata": map[string]interface{}{
				"name":      w.Workload.Name,
				"namespace": w.Application,
				"labels": map[string]interface{}{
					"radius.dev/application": w.Application,
					"radius.dev/component":   w.Name,
					// TODO get the component revision here...
					"app.kubernetes.io/name":       w.Name,
					"app.kubernetes.io/part-of":    w.Application,
					"app.kubernetes.io/managed-by": "radius-rp",
				},
			},

			// Config section is already in the right format
			"spec": w.Workload.Config,
		},
	}

	return []workloads.WorkloadResource{workloads.NewKubernetesResource("Component", &resource)}, nil
}
