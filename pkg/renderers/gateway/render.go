// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	gateway := radclient.GatewayProperties{}
	err := options.Resource.ConvertDefinition(&gateway)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	gatewayClassName := options.Runtime.Gateway.GatewayClass
	if gatewayClassName == "" {
		return renderers.RendererOutput{}, errors.New("gateway class not found")
	}

	publicIP := options.Runtime.Gateway.PublicIP
	if publicIP == "" {
		return renderers.RendererOutput{}, errors.New("public IP not found")
	}

	outputs := []outputresource.OutputResource{}

	gatewayName := kubernetes.MakeResourceName(options.Resource.ApplicationName, options.Resource.ResourceName)
	hostname, err := getHostname(ctx, options.Resource, gateway, publicIP)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("getting hostname failed with error: %s", err)
	}

	gatewayObject, err := MakeGateway(ctx, options.Resource, gateway, gatewayClassName, gatewayName, hostname)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputs = append(outputs, gatewayObject)

	computedValues := map[string]renderers.ComputedValueReference{
		"hostname": {
			Value: *hostname,
		},
	}

	httpRouteObjects, err := MakeHttpRoutes(options.Resource, gateway, gatewayName)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputs = append(outputs, httpRouteObjects...)

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func MakeGateway(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayClassName string, gatewayName string, hostname *gatewayv1alpha1.Hostname) (outputresource.OutputResource, error) {
	httpGateway, err := makeHttpGateway(ctx, resource, gateway, gatewayName, hostname)
	if err != nil {
		return outputresource.OutputResource{}, err
	}

	listeners := []gatewayv1alpha1.Listener{
		*httpGateway,
	}

	gatewayObject := &gatewayv1alpha1.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Gateway",
			APIVersion: gatewayv1alpha1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: gatewayv1alpha1.GatewaySpec{
			GatewayClassName: gatewayClassName,
			Listeners:        listeners,
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, gatewayObject, gatewayObject.ObjectMeta), nil
}

func MakeHttpRoutes(resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayName string) ([]outputresource.OutputResource, error) {
	var outputs []outputresource.OutputResource

	for _, route := range gateway.Routes {
		pathMatch := gatewayv1alpha1.PathMatchPrefix

		resourceID, err := azresources.Parse(*route.Destination)
		if err != nil {
			return []outputresource.OutputResource{}, nil
		}
		routeName := resourceID.Name()

		routeResourceName := kubernetes.MakeResourceName(resource.ApplicationName, routeName)
		port := gatewayv1alpha1.PortNumber(kubernetes.GetDefaultPort())

		rules := []gatewayv1alpha1.HTTPRouteRule{
			{
				Matches: []gatewayv1alpha1.HTTPRouteMatch{
					{
						Path: &gatewayv1alpha1.HTTPPathMatch{
							// Only support prefix-based path matching
							Type:  &pathMatch,
							Value: route.Path,
						},
					},
				},
				ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
					{
						Port:        &port,
						ServiceName: &routeResourceName,
					},
				},
			},
		}

		httpRouteObject := &gatewayv1alpha1.HTTPRoute{
			TypeMeta: metav1.TypeMeta{
				Kind:       "HTTPRoute",
				APIVersion: gatewayv1alpha1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeResourceName,
				Namespace: resource.ApplicationName,
				Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, routeName),
			},
			Spec: gatewayv1alpha1.HTTPRouteSpec{
				Gateways: &gatewayv1alpha1.RouteGateways{
					GatewayRefs: []gatewayv1alpha1.GatewayReference{
						{
							Name:      gatewayName,
							Namespace: "default",
						},
					},
				},
				Rules: rules,
			},
		}

		// Create unique localID for dependency graph
		localID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, routeName)

		outputs = append(outputs, outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, localID, httpRouteObject, httpRouteObject.ObjectMeta))
	}

	return outputs, nil
}

func makeHttpGateway(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayName string, hostname *gatewayv1alpha1.Hostname) (*gatewayv1alpha1.Listener, error) {
	port := kubernetes.GetDefaultPort()

	return &gatewayv1alpha1.Listener{
		Hostname: hostname,
		Port:     gatewayv1alpha1.PortNumber(port),
		Protocol: gatewayv1alpha1.HTTPProtocolType,
		Routes: gatewayv1alpha1.RouteBindingSelector{
			Kind: "HTTPRoute",
		},
	}, nil
}

func getHostname(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, publicIP string) (*gatewayv1alpha1.Hostname, error) {
	var hostname gatewayv1alpha1.Hostname
	if gateway.Hostname != nil {
		if gateway.Hostname.FullyQualifiedHostname != nil {
			// Use FQDN
			hostname = gatewayv1alpha1.Hostname(*gateway.Hostname.FullyQualifiedHostname)
		} else if gateway.Hostname.Prefix != nil {
			// Auto-assign hostname: prefix.appname.ip.nip.io
			prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", *gateway.Hostname.Prefix, resource.ApplicationName, publicIP)
			hostname = gatewayv1alpha1.Hostname(prefixedHostname)
		} else {
			return nil, errors.New("must provide either prefix or fullyQualifiedHostname if hostname is specified")
		}
	} else {
		// Auto-assign hostname: gatewayname.appname.ip.nip.io
		prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resource.ResourceName, resource.ApplicationName, publicIP)
		hostname = gatewayv1alpha1.Hostname(prefixedHostname)
	}

	return &hostname, nil
}
