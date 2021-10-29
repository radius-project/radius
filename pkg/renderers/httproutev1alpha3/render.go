// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import (
	"context"
	"fmt"

	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/gateway"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type Renderer struct {
}

// Need a step to take rendered routes to be usable by resource
func (r Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	route := HttpRoute{}
	err := resource.ConvertDefinition(&route)
	if err != nil {
		return nil, err
	}

	if route.Gateway != nil {
		resourceId, err := azresources.Parse(route.Gateway.Source)
		if err != nil {
			return nil, err
		}
		return []azresources.ResourceID{resourceId}, nil
	}
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	route := HttpRoute{}
	resource := options.Resource
	dependencies := options.Dependencies

	err := resource.ConvertDefinition(&route)
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

	service := r.makeService(resource, route)
	outputs = append(outputs, service)

	if route.Gateway != nil {
		gatewayId := route.Gateway.Source
		if gatewayId == "" {
			gatewayClass := options.AdditionalProperties[] // TODO
			gateway.MakeGateway(ctx, resource, route.Gateway, outputs)
		} else {
			existingGateway := dependencies[gatewayId]
			httpRoute := r.makeHttpRoute(resource, route, existingGateway)
			outputs = append(outputs, httpRoute)
		}
	}

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func (r *Renderer) makeService(resource renderers.RendererResource, route HttpRoute) outputresource.OutputResource {
	typeParts := strings.Split(resource.ResourceType, "/")
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: corev1.ServiceSpec{
			Selector: kubernetes.MakeRouteSelectorLabels(resource.ApplicationName, typeParts[len(typeParts)-1], resource.ResourceName),
			Type:     corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       resource.ResourceName,
					Port:       int32(route.GetEffectivePort()),
					TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + typeParts[len(typeParts)-1] + resource.ResourceName)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDService, service, service.ObjectMeta)
}

func (r *Renderer) makeHttpRoute(resource renderers.RendererResource, route HttpRoute, existingGateway renderers.RendererDependency) outputresource.OutputResource {

	serviceName := kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName)
	pathMatch := gatewayv1alpha1.PathMatchPrefix
	var rules []gatewayv1alpha1.HTTPRouteRule
	for _, rule := range route.Gateway.Rules {
		// Default to prefix match
		if strings.EqualFold(rule.Path.Type, "exact") {
			pathMatch = gatewayv1alpha1.PathMatchExact
		}
		port := gatewayv1alpha1.PortNumber(route.GetEffectivePort())
		rules = append(rules, gatewayv1alpha1.HTTPRouteRule{
			Matches: []gatewayv1alpha1.HTTPRouteMatch{
				{
					Path: &gatewayv1alpha1.HTTPPathMatch{
						Type:  &pathMatch,
						Value: &rule.Path.Value,
					},
				},
			},
			ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
				{
					ServiceName: &serviceName,
					Port:        &port,
				},
			},
		})
	}

	// Add a default rule which maps to the service if none specified
	if len(rules) == 0 {
		path := "/"
		port := gatewayv1alpha1.PortNumber(route.GetEffectivePort())
		rules = append(rules, gatewayv1alpha1.HTTPRouteRule{
			Matches: []gatewayv1alpha1.HTTPRouteMatch{
				{
					Path: &gatewayv1alpha1.HTTPPathMatch{
						Type:  &pathMatch,
						Value: &path,
					},
				},
			},
			ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
				{
					ServiceName: &serviceName,
					Port:        &port,
				},
			},
		})
	}
	var hostnames []gatewayv1alpha1.Hostname
	hostname := route.Gateway.Hostname
	if hostname != "" && hostname != "*" {
		hostnames = append(hostnames, gatewayv1alpha1.Hostname(hostname))
	}

	httpRoute := &gatewayv1alpha1.HTTPRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: gatewayv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: gatewayv1alpha1.HTTPRouteSpec{
			Gateways: &gatewayv1alpha1.RouteGateways{
				GatewayRefs: []gatewayv1alpha1.GatewayReference{
					{
						Name:      existingGateway.ResourceID.Name(),
						Namespace: "default",
					},
				},
			},
			Rules:     rules,
			Hostnames: hostnames,
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
}
