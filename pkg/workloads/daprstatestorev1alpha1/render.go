// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestorev1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/workloads"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Renderer is the WorkloadRenderer implementation for the dapr statestore workload.
type Renderer struct {
}

// Allocate is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "dapr.io/StateStore" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	// no values
	return map[string]interface{}{}, nil
}

// Render is the WorkloadRenderer implementation for dapr statestore workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	spec, err := getSpec(w.Workload)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if spec.Kind != "any" && spec.Kind != "state.azure.tablestorage" {
		return []workloads.WorkloadResource{}, errors.New("only kind 'any' and 'state.azure.tablestorage' is supported right now")
	}

	if !spec.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a state-store
	resource := workloads.WorkloadResource{
		Type: "dapr.statestore.azurestorage",
		Resource: map[string]string{
			"name":       w.Workload.GetName(),
			"namespace":  w.Workload.GetNamespace(),
			"apiVersion": "dapr.io/v1alpha1",
			"kind":       "Component",
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}

type stateStoreSpec struct {
	Kind    string `json:"kind"`
	Managed bool   `json:"managed"`
}

func getSpec(item unstructured.Unstructured) (stateStoreSpec, error) {
	spec, ok := item.Object["spec"]
	if !ok {
		return stateStoreSpec{}, errors.New("workload does not contain a spec element")
	}

	b, err := json.Marshal(spec)
	if err != nil {
		return stateStoreSpec{}, err
	}

	stateStore := stateStoreSpec{}
	err = json.Unmarshal(b, &stateStore)
	if err != nil {
		return stateStoreSpec{}, err
	}

	return stateStore, nil
}
