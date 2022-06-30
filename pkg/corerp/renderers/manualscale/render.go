// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscale

import (
	"context"
	"errors"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Renderer is the renderers.Renderer implementation for the manualscale extension.
type Renderer struct {
	Inner renderers.Renderer
}

// GetDependencyIDs returns dependencies for the container/other datamodel passed in
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	// Let the inner renderer do its work
	return r.Inner.GetDependencyIDs(ctx, resource)
}

// Render augments the container's kubernetes output resource with value for manualscale replica if applicable.
func (r *Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, dm, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resource, ok := dm.(datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	extensions := resource.Properties.Extensions

	for _, e := range extensions {
		switch e.GetExtension().Kind {
		case Kind:
			for _, ores := range output.Resources {
				if ores.ResourceType.Provider != providers.ProviderKubernetes {
					// Not a Kubernetes resource
					continue
				}
				o, ok := ores.Resource.(runtime.Object)
				if !ok {
					return renderers.RendererOutput{}, errors.New("found Kubernetes resource with non-Kubernetes payload")
				}
				ext, ok := e.(datamodel.ManualScalingExtension)
				if !ok {
					return renderers.RendererOutput{}, errors.New("error rendering ManualScale Extension")
				}
				if ext.Replicas != nil {
					r.setReplicas(o, ext.Replicas)
				}
			}
		default:
			continue
		}
		break
	}

	return output, nil
}

// setReplicas sets the value of replica
func (r Renderer) setReplicas(o runtime.Object, replicas *int32) {
	if dep, ok := o.(*appsv1.Deployment); ok {
		dep.Spec.Replicas = replicas
	}
}
