// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprcomponentv1alpha1

import (
	"context"
	"fmt"

	"github.com/Azure/radius/pkg/workloads"
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
	// It's already in the correct format
	return []workloads.WorkloadResource{workloads.NewKubernetesResource("Component", &w.Workload)}, nil
}
