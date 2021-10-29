// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/renderers/daprhttproutev1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
	Inner renderers.Renderer
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	dependencies, err := r.Inner.GetDependencyIDs(ctx, resource)
	if err != nil {
		return nil, err
	}

	trait, err := r.FindTrait(resource)
	if err != nil {
		return nil, err
	}

	if trait == nil {
		return dependencies, nil
	}

	if trait.Provides == "" {
		return dependencies, nil
	}

	parsed, err := azresources.Parse(trait.Provides)
	if err != nil {
		return nil, err
	}

	return append(dependencies, parsed), nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	output, err := r.Inner.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	if err != nil {
		return renderers.RendererOutput{}, nil
	}

	trait, err := r.FindTrait(resource)
	if err != nil {
		return renderers.RendererOutput{}, nil
	}

	if trait == nil {
		return output, nil
	}

	// If we get here then we found a Dapr Sidecar trait. We need to update the Kubernetes deployment with
	// the desired annotations.

	// Resolve the AppID:
	// 1. If there's a DaprHttpRoute then it *must* specify an app id.
	// 2. The trait specifies an app id (must not conflict with 1)
	// 3. (none)

	appID, err := r.resolveAppId(*trait, dependencies)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	for i := range output.Resources {
		if output.Resources[i].ResourceKind != resourcekinds.Kubernetes {
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

		if appID != "" {
			annotations["dapr.io/app-id"] = appID
		}
		if trait.AppPort != 0 {
			annotations["dapr.io/app-port"] = fmt.Sprintf("%d", trait.AppPort)
		}
		if trait.Config != "" {
			annotations["dapr.io/config"] = trait.Config
		}
		if trait.Protocol != "" {
			annotations["dapr.io/protocol"] = trait.Protocol
		}

		r.setAnnotations(o, annotations)
	}

	return output, nil
}

func (r *Renderer) FindTrait(resource renderers.RendererResource) (*Trait, error) {
	container := containerv1alpha3.ContainerProperties{}
	err := resource.ConvertDefinition(&container)
	if err != nil {
		return nil, err
	}

	trait := Trait{}
	found, err := container.FindTrait(Kind, &trait)
	if err != nil {
		return nil, err
	} else if !found {
		return nil, nil
	}

	return &trait, nil
}

func (r *Renderer) resolveAppId(trait Trait, dependencies map[string]renderers.RendererDependency) (string, error) {
	// We're being extra pedantic here about reporting error cases. None of these
	// cases should be possible to trigger with user input, they would result from internal bugs.
	routeAppID := ""
	if trait.Provides != "" {
		routeDependency, ok := dependencies[trait.Provides]
		if !ok {
			return "", fmt.Errorf("failed to find depenendency with id %q", trait.Provides)
		}

		route := daprhttproutev1alpha3.DaprHttpRouteProperties{}
		err := routeDependency.ConvertDefinition(&route)
		if err != nil {
			return "", err
		}

		routeAppID = route.AppID
	}

	if trait.AppID != "" && routeAppID != "" && trait.AppID != routeAppID {
		return "", fmt.Errorf("the appId specified on a %q must match the appId specified on the %q trait. Route: %q, Trait: %q", daprhttproutev1alpha3.ResourceType, Kind, routeAppID, trait.AppID)
	}

	if routeAppID != "" {
		return routeAppID, nil
	}

	return trait.AppID, nil
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
