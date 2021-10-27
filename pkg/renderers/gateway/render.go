// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type Renderer struct {
	Client client.Client
}

// Need a step to take rendered routes to be usable by resource
func (r Renderer) GetDependencyIDs(Clientctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	gateway := Gateway{}
	err := resource.ConvertDefinition(&gateway)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	// We require a gateway class to be present before creating a gateway
	// Look up the first gateway class in the cluster and use that for now
	var gateways gatewayv1alpha1.GatewayClassList
	err = r.Client.List(ctx, &gateways)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if len(gateways.Items) == 0 {
		return renderers.RendererOutput{}, errors.New("no gateway classes found")
	}

	gatewayClass := gateways.Items[0]

	computedValues := map[string]renderers.ComputedValueReference{}

	outputs := []outputresource.OutputResource{}
	outputs = append(outputs, r.makeGateway(ctx, resource, gateway, gatewayClass))

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeGateway(ctx context.Context, resource renderers.RendererResource, gateway Gateway, gatewayClass gatewayv1alpha1.GatewayClass) outputresource.OutputResource {
	var listeners []gatewayv1alpha1.Listener
	for _, listener := range gateway.Listeners {
		listeners = append(listeners, gatewayv1alpha1.Listener{
			Port:     gatewayv1alpha1.PortNumber(*listener.Port),
			Protocol: gatewayv1alpha1.ProtocolType(listener.Protocol),
			Routes: gatewayv1alpha1.RouteBindingSelector{
				Kind: "HTTPRoute",
			},
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
			GatewayClassName: gatewayClass.Name,
			Listeners:        listeners,
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDGateway, gate, gate.ObjectMeta)
}
