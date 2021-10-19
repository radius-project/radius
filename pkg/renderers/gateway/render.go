// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"

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

	computedValues := map[string]renderers.ComputedValueReference{}

	outputs := []outputresource.OutputResource{}
	// TODO can't specify listener here as ingress doesn't allow you to specify port to listen on, should we just remove?
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
	ingress.Spec = networkingv1.IngressSpec{
		DefaultBackend: &networkingv1.IngressBackend{},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDIngress, ingress, ingress.ObjectMeta)
}
