// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type Renderer struct {
}

const (
	GatewayClassKey = "GatewayClass"
)

func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	gateway := Gateway{}
	err := options.Resource.ConvertDefinition(&gateway)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	gatewayClassName := options.Runtime.Gateway.GatewayClass
	if gatewayClassName == "" {
		return renderers.RendererOutput{}, errors.New("gateway class not found")
	}
	computedValues := map[string]renderers.ComputedValueReference{}

	outputs := []outputresource.OutputResource{}
	outputs = append(outputs, MakeGateway(ctx, options.Resource, gateway, gatewayClassName))

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func MakeGateway(ctx context.Context, resource renderers.RendererResource, gateway Gateway, gatewayClassName string) outputresource.OutputResource {
	var listeners []gatewayv1alpha2.Listener
	for key, listener := range gateway.Listeners {
		listeners = append(listeners, gatewayv1alpha2.Listener{
			Name:     gatewayv1alpha2.SectionName(key),
			Port:     gatewayv1alpha2.PortNumber(*listener.Port),
			Protocol: gatewayv1alpha2.ProtocolType(listener.Protocol),
			AllowedRoutes: &gatewayv1alpha2.AllowedRoutes{
				Kinds: []gatewayv1alpha2.RouteGroupKind{
					{
						Kind: "HTTPRoute",
					},
				},
			},
		})
	}

	gate := &gatewayv1alpha2.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: gatewayv1alpha2.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: gatewayv1alpha2.GatewaySpec{
			GatewayClassName: gatewayv1alpha2.ObjectName(gatewayClassName),
			Listeners:        listeners,
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDGateway, gate, gate.ObjectMeta)
}
