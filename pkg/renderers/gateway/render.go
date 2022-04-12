// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

const (
	GatewayClassKey = "GatewayClass"
)

func (r Renderer) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	// TODO: willsmith: may need to create routes first
	// maybe just get ID and use it to create resourcename, reference
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
	computedValues := map[string]renderers.ComputedValueReference{}

	outputs := []outputresource.OutputResource{}

	gatewayName := kubernetes.MakeResourceName(options.Resource.ApplicationName, options.Resource.ResourceName)
	gatewayObject, err := MakeGateway(ctx, options.Resource, gateway, gatewayClassName, gatewayName)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputs = append(outputs, gatewayObject)

	httpRouteObjects := MakeHttpRoutes(options.Resource, gateway, gatewayName)
	outputs = append(outputs, httpRouteObjects...)

	return renderers.RendererOutput{
		Resources:      outputs,
		ComputedValues: computedValues,
	}, nil
}

func MakeGateway(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayClassName string, gatewayName string) (outputresource.OutputResource, error) {
	httpGateway, err := makeHttpGateway(ctx, resource, gateway, gatewayName)
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

func MakeHttpRoutes(resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayName string) []outputresource.OutputResource {
	var outputs []outputresource.OutputResource

	for _, route := range gateway.Routes {
		pathMatch := gatewayv1alpha1.PathMatchPrefix
		routeName := kubernetes.MakeResourceName(resource.ApplicationName, getRouteNameFromID(*route.Destination))
		port := gatewayv1alpha1.PortNumber(80)

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
						ServiceName: &routeName,
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
				// TODO: willsmith: resource name isn't right here
				Name:      kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName),
				Namespace: resource.ApplicationName,
				Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
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

		outputs = append(outputs, outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, outputresource.LocalIDHttpRoute, httpRouteObject, httpRouteObject.ObjectMeta))
	}

	return outputs
}

func makeHttpGateway(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayName string) (*gatewayv1alpha1.Listener, error) {
	port := 80

	var hostname gatewayv1alpha1.Hostname
	if gateway.Hostname != nil {
		if gateway.Hostname.FullyQualifiedHostname != nil {
			// Use FQDN
			hostname = gatewayv1alpha1.Hostname(*gateway.Hostname.FullyQualifiedHostname)
		} else if gateway.Hostname.Prefix != nil {
			// Auto-assign hostname: prefix.appname.ip.nip.io
			endpoint, err := getPublicEndpoint(ctx)
			if err != nil {
				return nil, err
			}

			prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", *gateway.Hostname.Prefix, resource.ApplicationName, *endpoint)
			hostname = gatewayv1alpha1.Hostname(prefixedHostname)
		} else {
			return nil, errors.New("must provide either prefix or fullyQualifiedHostname if hostname is specified")
		}
	} else {
		// Auto-assign hostname: gatewayname.appname.ip.nip.io
		endpoint, err := getPublicEndpoint(ctx)
		if err != nil {
			return nil, err
		}

		prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resource.ResourceName, resource.ApplicationName, *endpoint)
		hostname = gatewayv1alpha1.Hostname(prefixedHostname)
	}

	return &gatewayv1alpha1.Listener{
		Hostname: &hostname,
		Port:     gatewayv1alpha1.PortNumber(port),
		Protocol: gatewayv1alpha1.HTTPProtocolType,
		Routes: gatewayv1alpha1.RouteBindingSelector{
			Kind: "HTTPRoute",
		},
	}, nil
}

func getPublicEndpoint(ctx context.Context) (*string, error) {
	client, err := kubernetes.NewKubernetesClient()
	if err != nil {
		return nil, err
	}

	return client.GetPublicIP(ctx)
}

func getRouteNameFromID(routeID string) string {
	splitString := strings.Split(routeID, "/")
	if len(splitString) == 0 {
		return ""
	}

	return splitString[len(splitString)-1]
}
