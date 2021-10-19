// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"fmt"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
)

type Renderer struct {
}

// Need a step to take rendered routes to be usable by resource
func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	gateway := Gateway{}
	err := resource.ConvertDefinition(&gateway)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
		},
		"port": {
			Value: route.GetEffectivePort(),
		},
		"url": {
			Value: fmt.Sprintf("http://%s:%d", kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), route.GetEffectivePort()),
		},
		"scheme": {
			Value: "http",
		},
	}

	outputs := []outputresource.OutputResource{}

	ingress := r.makeIngress(resource, gateway)
	outputs = append(outputs, ingress)

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeIngress(resource renderers.RendererResource, gateway Gateway) outputresource.OutputResource {
	ingress := &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: networkingv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
	}

	backend := networkingv1.IngressBackend{
		Service: &networkingv1.IngressServiceBackend{
			Name: kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Port: networkingv1.ServiceBackendPort{
				Number: int32(route.GetEffectivePort()),
			},
		},
	}

	// Default path to / if not specified
	path := route.Gateway.Path
	if path == "" {
		path = "/"
	}

	var defaultBackend *networkingv1.IngressBackend
	host := route.Gateway.Hostname
	if route.Gateway.Hostname == "*" {
		defaultBackend = &backend
		// * isn't allowed in the hostname, remove it.
		host = ""
	}
	pathType := networkingv1.PathTypePrefix

	spec := networkingv1.IngressSpec{
		DefaultBackend: defaultBackend,
		Rules: []networkingv1.IngressRule{
			{
				Host: host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     path,
								PathType: &pathType,
								Backend:  backend,
							},
						},
					},
				},
			},
		},
	}

	ingress.Spec = spec

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDIngress, ingress, ingress.ObjectMeta)
}
