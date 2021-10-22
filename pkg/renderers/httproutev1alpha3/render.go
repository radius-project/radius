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
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type Renderer struct {
}

// Need a step to take rendered routes to be usable by resource
func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	route := HttpRoute{}
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
		// If not source specified, create an ingress by default.
		gatewayId := route.Gateway.Source
		if gatewayId != "" {
			// TODO: dependency doesn't have ingress here
			existingIngress := dependencies[gatewayId]
			ingress := r.makeIngressRule(resource, route, existingIngress)
			outputs = append(outputs, ingress)
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

func (r *Renderer) makeIngressRule(resource renderers.RendererResource, route HttpRoute, existingIngress renderers.RendererDependency) outputresource.OutputResource {
	// gatewayName := kubernetes.MakeResourceName(resource.ApplicationName, existingIngress.ResourceID.Name())
	serviceName := kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName)
	var rules []gatewayv1alpha1.HTTPRouteRule
	for _, rule := range route.Gateway.Rules {
		pathMatch := gatewayv1alpha1.PathMatchPrefix
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
						Name:      "gateway",
						Namespace: "default",
					},
				},
			},
			Rules: rules,
		},
	}

	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
}

// // Instead of making the ingress here, we need to get the previous ingress and update it
// func (r *Renderer) makeIngress(resource renderers.RendererResource, route HttpRoute) outputresource.OutputResource {
// 	ingress := &networkingv1.Ingress{
// 		TypeMeta: metav1.TypeMeta{
// 			Kind:       "Ingress",
// 			APIVersion: networkingv1.SchemeGroupVersion.String(),
// 		},
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
// 			Namespace: resource.ApplicationName,
// 			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
// 		},
// 	}

// 	backend := networkingv1.IngressBackend{
// 		Service: &networkingv1.IngressServiceBackend{
// 			Name: kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
// 			Port: networkingv1.ServiceBackendPort{
// 				Number: int32(route.GetEffectivePort()),
// 			},
// 		},
// 	}

// 	// Default path to / if not specified
// 	path := route.Gateway.Path
// 	if path == "" {
// 		path = "/"
// 	}

// 	var defaultBackend *networkingv1.IngressBackend
// 	host := route.Gateway.Hostname
// 	if route.Gateway.Hostname == "*" {
// 		defaultBackend = &backend
// 		// * isn't allowed in the hostname, remove it.
// 		host = ""
// 	}
// 	pathType := networkingv1.PathTypePrefix

// 	spec := networkingv1.IngressSpec{
// 		DefaultBackend: defaultBackend,
// 		Rules: []networkingv1.IngressRule{
// 			{
// 				Host: host,
// 				IngressRuleValue: networkingv1.IngressRuleValue{
// 					HTTP: &networkingv1.HTTPIngressRuleValue{
// 						Paths: []networkingv1.HTTPIngressPath{
// 							{
// 								Path:     path,
// 								PathType: &pathType,
// 								Backend:  backend,
// 							},
// 						},
// 					},
// 				},
// 			},
// 		},
// 	}

// 	ingress.Spec = spec

// 	return outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, ingress, ingress.ObjectMeta)
// }
