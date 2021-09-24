// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	context "context"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/workloads"
)

// Defines an interface for renderers that can work with AppModelv3 through an adapter.
//
// These workloads have the following limitations:
// - No support for 'run', 'uses', 'traits'
// - No user-defined 'bindings'
// - No support for creating resources during rendering
//
// Generally this is our non-runnable components
type AdaptableRenderer interface {
	workloads.WorkloadRenderer

	// Refers to the renderer v1 kind, we need to pass this through to the renderer in case it validates it.
	GetKind() string

	GetComputedValues(ctx context.Context, workload workloads.InstantiatedWorkload) (map[string]ComputedValueReference, map[string]SecretValueReference, error)
}

var _ Renderer = (*V1RendererAdapter)(nil)

type V1RendererAdapter struct {
	Inner AdaptableRenderer
}

func (r *V1RendererAdapter) GetDependencyIDs(ctx context.Context, resource RendererResource) ([]azresources.ResourceID, error) {
	// Implementations that use this adapter do not support dependencies
	return nil, nil
}

func (r *V1RendererAdapter) Render(ctx context.Context, resource RendererResource, dependencies map[string]RendererDependency) (RendererOutput, error) {
	// Our task here is to call into the wrapper render function and then interpret the results into our new format.

	// The supported component types only use the 'config' section in our old format (see comments on AdaptableRenderer)
	workload := workloads.InstantiatedWorkload{
		Application: resource.ApplicationName,
		Name:        resource.ResourceName,
		Workload: components.GenericComponent{
			Name:     resource.ResourceName,
			Kind:     r.Inner.GetKind(),
			Config:   resource.Definition,
			Run:      map[string]interface{}{},
			Bindings: map[string]components.GenericBinding{},
			Uses:     []components.GenericDependency{},
			Traits:   []components.GenericTrait{},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := r.Inner.Render(ctx, workload)
	if err != nil {
		return RendererOutput{}, err
	}

	computedValues, secretValues, err := r.Inner.GetComputedValues(ctx, workload)
	if err != nil {
		return RendererOutput{}, err
	}

	return RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}
