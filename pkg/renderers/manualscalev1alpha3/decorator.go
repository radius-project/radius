// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscalev1alpha3

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ renderers.Renderer = (*Renderer)(nil)

// Renderer is the renderers.Renderer implementation for the manualscale trait decorator.
type Renderer struct {
	Inner renderers.Renderer
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	// Let the inner renderer do its work
	return r.Inner.GetDependencyIDs(ctx, resource)
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, options)
	if err != nil {
		return renderers.RendererOutput{}, nil
	}
	resource := options.Resource

	container := radclient.ContainerProperties{}
	err = resource.ConvertDefinition(&container)
	if err != nil {
		return renderers.RendererOutput{}, nil
	}

	for _, t := range container.Traits {
		switch trait := t.(type) {
		case *radclient.ManualScalingTrait:
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
		default:
			continue
		}
		break
	}

	return output, nil
}

func (r Renderer) setReplicas(o runtime.Object, replicas *int32) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		dep.Spec.Replicas = replicas
	}
}
