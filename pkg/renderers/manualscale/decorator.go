// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscale

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Renderer is the WorkloadRenderer implementation for the manualscale trait decorator.
type Renderer struct {
	Inner workloads.WorkloadRenderer
}

// Allocate is the WorkloadRenderer implementation for the manualscale trait decorator.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	// ManualScale doesn't affect bindings today as the binding returned by the container
	// workload references the k8s hostname, which will do round-robin load balancing by default.
	return r.Inner.AllocateBindings(ctx, workload, resources)
}

// Render is the WorkloadRenderer implementation for the manualscale deployment decorator.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	// Let the inner renderer do its work
	resources, err := r.Inner.Render(ctx, w)
	if err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		// See: https://github.com/Azure/radius/issues/499
		return resources, err
	}

	trait := Trait{}
	if found, err := w.Workload.FindTrait(Kind, &trait); !found || err != nil {
		// Even if the operation fails, return the output resources created so far
		// TODO: This is temporary. Once there are no resources actually deployed during render phase,
		// we no longer need to track the output resources on error
		// See: https://github.com/Azure/radius/issues/499
		return resources, err
	}

	// ManualScaling detected, update deployment
	for _, resource := range resources {
		if resource.Kind != resourcekinds.KindKubernetes {
			// Not a Kubernetes resource
			continue
		}

		o, ok := resource.Resource.(runtime.Object)
		if !ok {
			// Even if the operation fails, return the output resources created so far
			// TODO: This is temporary. Once there are no resources actually deployed during render phase,
			// we no longer need to track the output resources on error
			// See: https://github.com/Azure/radius/issues/499
			return resources, errors.New("found Kubernetes resource with non-Kubernetes payload")
		}

		if trait.Replicas != nil {
			r.setReplicas(o, trait.Replicas)
		}
	}

	return resources, err
}

func (r Renderer) setReplicas(o runtime.Object, replicas *int32) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		dep.Spec.Replicas = replicas
	}
}
