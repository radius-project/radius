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

package daprextension

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// Renderer is the renderers.Renderer implementation for the dapr sidecar extension.
type Renderer struct {
	Inner renderers.Renderer
}

// GetDependencyIDs returns dependencies for the container datamodel passed in
func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	radiusDependencyIDs, azureDependencyIDs, err := r.Inner.GetDependencyIDs(ctx, dm)
	if err != nil {
		return nil, nil, err
	}

	return radiusDependencyIDs, azureDependencyIDs, nil
}

// Render augments the container's kubernetes output resource with value for dapr sidecar extension.
func (r *Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	output, err := r.Inner.Render(ctx, resource, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	extension, err := r.findExtension(resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if extension == nil {
		return output, nil
	}

	// If we get here then we found a Dapr Sidecar extension. We need to update the Kubernetes deployment with
	// the desired annotations.

	for i := range output.Resources {
		if output.Resources[i].ResourceType.Provider != resourcemodel.ProviderKubernetes {
			// Not a Kubernetes resource
			continue
		}

		o, ok := output.Resources[i].Resource.(runtime.Object)
		if !ok {
			return renderers.RendererOutput{}, errors.New("found Kubernetes resource with non-Kubernetes payload")
		}

		annotations, ok := r.getAnnotations(o)
		if !ok {
			continue
		}

		annotations["dapr.io/enabled"] = "true"

		if extension.AppID != "" {
			annotations["dapr.io/app-id"] = extension.AppID
		}
		if appPort := extension.AppPort; appPort != 0 {
			annotations["dapr.io/app-port"] = fmt.Sprintf("%d", appPort)
		}
		if config := extension.Config; config != "" {
			annotations["dapr.io/config"] = config
		}
		if extension.Protocol != "" {
			annotations["dapr.io/protocol"] = string(extension.Protocol)
		}

		r.setAnnotations(o, annotations)
	}

	return output, nil
}

func (r *Renderer) findExtension(dm v1.DataModelInterface) (*datamodel.DaprSidecarExtension, error) {
	container, ok := dm.(*datamodel.ContainerResource)
	if !ok {
		return nil, v1.ErrInvalidModelConversion
	}

	for _, t := range container.Properties.Extensions {
		switch t.Kind {
		case datamodel.DaprSidecar:
			return t.DaprSidecar, nil
		}
	}
	return nil, nil
}

func (r *Renderer) getAnnotations(o runtime.Object) (map[string]string, bool) {
	dep, ok := o.(*appsv1.Deployment)
	if ok {
		if dep.Spec.Template.Annotations == nil {
			dep.Spec.Template.Annotations = map[string]string{}
		}

		return dep.Spec.Template.Annotations, true
	}

	un, ok := o.(*unstructured.Unstructured)
	if ok {
		if a := un.GetAnnotations(); a != nil {
			return a, true
		}

		return map[string]string{}, true
	}

	return nil, false
}

func (r *Renderer) setAnnotations(o runtime.Object, annotations map[string]string) {
	un, ok := o.(*unstructured.Unstructured)
	if ok {
		un.SetAnnotations(annotations)
	}
}
