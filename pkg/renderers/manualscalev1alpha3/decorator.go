// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscalev1alpha3

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ renderers.Renderer = (*Renderer)(nil)

// Renderer is the renderers.Renderer implementation for the manualscale trait decorator.
type Renderer struct {
	Inner renderers.Renderer
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	// Let the inner renderer do its work
	return r.Inner.GetDependencyIDs(ctx, resource)
}

func (r *Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, resource, dependencies)
	if err != nil {
		return renderers.RendererOutput{}, nil
	}

	container := containerv1alpha3.ContainerProperties{}
	err = resource.ConvertDefinition(&container)
	if err != nil {
		return renderers.RendererOutput{}, nil
	}

	trait := Trait{}
	found, err := container.FindTrait(Kind, &trait)
	if err != nil {
		return renderers.RendererOutput{}, nil
	} else if !found {
		return output, nil
	}

	// ManualScaling detected, update deployment
	for _, resource := range output.Resources {
		if resource.ResourceKind != resourcekinds.Kubernetes {
			// Not a Kubernetes resource
			continue
		}

		o, ok := resource.Resource.(runtime.Object)
		if !ok {
			return renderers.RendererOutput{}, errors.New("found Kubernetes resource with non-Kubernetes payload")
		}

		if trait.Replicas != nil {
			r.setReplicas(o, trait.Replicas)
		}
	}

	return output, nil
}

func (r Renderer) setReplicas(o runtime.Object, replicas *int32) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		dep.Spec.Replicas = replicas
	}
}
