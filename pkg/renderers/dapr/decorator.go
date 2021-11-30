// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/Azure/radius/pkg/renderers"
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

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	dependencies, _, err := r.Inner.GetDependencyIDs(ctx, resource)
	if err != nil {
		return nil, nil, err
	}

	trait, err := r.FindTrait(resource)
	if err != nil {
		return nil, nil, err
	}

	if trait == nil {
		return dependencies, nil, nil
	}

	provides := to.String(trait.Provides)
	if provides == "" {
		return dependencies, nil, nil
	}

	parsed, err := azresources.Parse(provides)
	if err != nil {
		return nil, nil, err
	}

	return append(dependencies, parsed), nil, nil
}

func (r *Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource := options.Resource
	dependencies := options.Dependencies
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
		if appPort := to.Int32(trait.AppPort); appPort != 0 {
			annotations["dapr.io/app-port"] = fmt.Sprintf("%d", appPort)
		}
		if config := to.String(trait.Config); config != "" {
			annotations["dapr.io/config"] = config
		}
		if trait.Protocol != nil {
			annotations["dapr.io/protocol"] = string(*trait.Protocol)
		}

		r.setAnnotations(o, annotations)
	}

	return output, nil
}

func (r *Renderer) FindTrait(resource renderers.RendererResource) (*radclient.DaprSidecarTrait, error) {
	container := radclient.ContainerComponentProperties{}
	err := resource.ConvertDefinition(&container)
	if err != nil {
		return nil, err
	}

	for _, t := range container.Traits {
		switch trait := t.(type) {
		case *radclient.DaprSidecarTrait:
			return trait, nil
		}
	}
	return nil, nil
}

func (r *Renderer) resolveAppId(trait radclient.DaprSidecarTrait, dependencies map[string]renderers.RendererDependency) (string, error) {
	// We're being extra pedantic here about reporting error cases. None of these
	// cases should be possible to trigger with user input, they would result from internal bugs.
	routeAppID := ""
	if provides := to.String(trait.Provides); provides != "" {
		routeDependency, ok := dependencies[provides]
		if !ok {
			return "", fmt.Errorf("failed to find depenendency with id %q", provides)
		}

		route := radclient.DaprHTTPRouteProperties{}
		err := routeDependency.ConvertDefinition(&route)
		if err != nil {
			return "", err
		}
		routeAppID = to.String(route.AppID)
	}

	appID := to.String(trait.AppID)
	if appID != "" && routeAppID != "" && appID != routeAppID {
		return "", fmt.Errorf("the appId specified on a %q must match the appId specified on the %q trait. Route: %q, Trait: %q", daprhttproutev1alpha3.ResourceType, *trait.Kind, routeAppID, appID)
	}

	if routeAppID != "" {
		return routeAppID, nil
	}

	return appID, nil
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
