// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the dapr statestore workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	bindings := map[string]components.BindingState{
		"default": {
			Component: workload.Name,
			Binding:   "default",
			Properties: map[string]interface{}{
				"stateStoreName": workload.Name,
			},
		},
	}

	return bindings, nil
}

// Render is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := DaprStateStoreComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if component.Config.Kind != "any" && component.Config.Kind != "state.azure.tablestorage" {
		return []workloads.WorkloadResource{}, errors.New("only kind 'any' and 'state.azure.tablestorage' is supported right now")
	}

	if !component.Config.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a state-store
	resource := workloads.WorkloadResource{
		Type: workloads.ResourceKindDaprStateStoreAzureStorage,
		Resource: map[string]string{
			handlers.ManagedKey:                "true",
			handlers.KubernetesNameKey:         w.Name,
			handlers.KubernetesNamespaceKey:    w.Application,
			handlers.KubernetesAPIVersionKey:   "dapr.io/v1alpha1",
			handlers.KubernetesKindKey:         "Component",
			handlers.StorageAccountBaseNameKey: w.Name,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}
