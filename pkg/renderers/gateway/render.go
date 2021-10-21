// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
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

	computedValues := map[string]renderers.ComputedValueReference{} // TODO add computed values

	outputs := []outputresource.OutputResource{}
	outputs = append(outputs, r.makeGateway(resource, gateway))

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeGateway(resource renderers.RendererResource, gateway Gateway) outputresource.OutputResource {
	var listeners []gatewayv1alpha1.Listener
	for _, listener := range gateway.Listeners {
		listeners = append(listeners, gatewayv1alpha1.Listener{
			Port:     gatewayv1alpha1.PortNumber(*listener.Port),
			Protocol: gatewayv1alpha1.ProtocolType(listener.Protocol),
		})
	}

	gate := &gatewayv1alpha1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: gatewayv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: gatewayv1alpha1.GatewaySpec{
			GatewayClassName: "foo", // for some reason this is required.
			Listeners:        listeners,
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDGateway, gate, gate.ObjectMeta)
}
