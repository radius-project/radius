/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package manualscale

import (
	"context"
	"errors"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/resourcemodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Renderer is the renderers.Renderer implementation for the manualscale extension.
type Renderer struct {
	Inner renderers.Renderer
}

// GetDependencyIDs gets the IDs of the dependencies of the given resource.
func (r *Renderer) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	// Let the inner renderer do its work
	return r.Inner.GetDependencyIDs(ctx, resource)
}

// Render checks if the DataModelInterface is a ContainerResource and if so, checks for ManualScaling
// extensions and sets the replicas accordingly.
func (r *Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Let the inner renderer do its work
	output, err := r.Inner.Render(ctx, dm, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	extensions := resource.Properties.Extensions
	for _, e := range extensions {
		switch e.Kind {
		case datamodel.ManualScaling:
			for _, ores := range output.Resources {
				resourceType := ores.GetResourceType()
				if resourceType.Provider != resourcemodel.ProviderKubernetes {
					// Not a Kubernetes resource
					continue
				}
				o, ok := ores.CreateResource.Data.(runtime.Object)
				if !ok {
					return renderers.RendererOutput{}, errors.New("found Kubernetes resource with non-Kubernetes payload")
				}

				if e.ManualScaling != nil && e.ManualScaling.Replicas != nil {
					r.setReplicas(o, e.ManualScaling.Replicas)
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
