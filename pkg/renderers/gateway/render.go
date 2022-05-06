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
	"github.com/project-radius/radius/pkg/environment"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
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

	fmt.Printf("RuntimeOptions: %+v\n", options.Runtime)

	outputs := []outputresource.OutputResource{}

	gatewayName := kubernetes.MakeResourceName(options.Resource.ApplicationName, options.Resource.ResourceName)
	hostname, err := getHostname(ctx, options.Resource, gateway, options.Runtime)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("getting hostname failed with error: %s", err)
	}

	gatewayObject, err := MakeGateway(ctx, options.Resource, gateway, gatewayName, hostname)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputs = append(outputs, gatewayObject)

	var computedHostname string
	if hostname == nil {
		computedHostname = "unknown"
	} else {
		computedHostname = string(*hostname)
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"hostname": {
			Value: computedHostname,
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

func MakeGateway(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayName string, hostname *string) (outputresource.OutputResource, error) {
	includes := []contourv1.Include{}

	if len(gateway.Routes) < 1 {
		return outputresource.OutputResource{}, errors.New("must have at least one route when declaring a Gateway resource")
	}

	for _, route := range gateway.Routes {
		routeName, err := getRouteName(route)
		if err != nil {
			return outputresource.OutputResource{}, err
		}

		routeResourceName := kubernetes.MakeResourceName(resource.ApplicationName, routeName)

		includes = append(includes, contourv1.Include{
			Name: routeResourceName,
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: *route.Path,
				},
			},
		})
	}

	var virtualHost *contourv1.VirtualHost = nil
	if hostname != nil {
		virtualHost = &contourv1.VirtualHost{
			Fqdn: *hostname,
		}
	}

	// The root HTTPProxy object acts as the Gateway
	rootHTTPProxy := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: contourv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, resource.ResourceName),
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: virtualHost,
			Includes:    includes,
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, rootHTTPProxy, rootHTTPProxy.ObjectMeta), nil
}

func MakeHttpRoutes(resource renderers.RendererResource, gateway radclient.GatewayProperties, gatewayName string) ([]outputresource.OutputResource, error) {
	var outputs []outputresource.OutputResource

	for _, route := range gateway.Routes {
		routeName, err := getRouteName(route)
		if err != nil {
			return []outputresource.OutputResource{}, err
		}

		routeResourceName := kubernetes.MakeResourceName(resource.ApplicationName, routeName)
		port := kubernetes.GetDefaultPort()

		httpProxyObject := &contourv1.HTTPProxy{
			TypeMeta: metav1.TypeMeta{
				Kind:       "HTTPProxy",
				APIVersion: contourv1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeResourceName,
				Namespace: resource.ApplicationName,
				Labels:    kubernetes.MakeDescriptiveLabels(resource.ApplicationName, routeName),
			},
			Spec: contourv1.HTTPProxySpec{
				Routes: []contourv1.Route{
					{
						Services: []contourv1.Service{
							{
								Name: routeResourceName,
								Port: port,
							},
						},
					},
				},
			},
		}

		// Create unique localID for dependency graph
		localID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, routeName)

		outputs = append(outputs, outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, localID, httpProxyObject, httpProxyObject.ObjectMeta))
	}

	return outputs, nil
}

func getRouteName(route *radclient.GatewayRoute) (string, error) {
	resourceID, err := azresources.Parse(*route.Destination)
	if err != nil {
		return "", err
	}

	return resourceID.Name(), nil
}

func getHostname(ctx context.Context, resource renderers.RendererResource, gateway radclient.GatewayProperties, options renderers.RuntimeOptions) (*string, error) {
	publicIP := options.Gateway.PublicIP

	if publicIP == "" {
		// In the case of no publicIP, use the application name as the hostname
		return &resource.ApplicationName, nil
	} else if options.Environment == environment.KindDev {
		// Use the local dev http endpoint
		return &options.Gateway.PublicIP, nil
	} else if gateway.Hostname != nil {
		if gateway.Hostname.FullyQualifiedHostname != nil {
			// Use FQDN
			return gateway.Hostname.FullyQualifiedHostname, nil
		} else if gateway.Hostname.Prefix != nil {
			// Auto-assign hostname: prefix.appname.ip.nip.io
			prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", *gateway.Hostname.Prefix, resource.ApplicationName, publicIP)
			return &prefixedHostname, nil
		} else {
			return nil, errors.New("must provide either prefix or fullyQualifiedHostname if hostname is specified")
		}
	} else {
		// Auto-assign hostname: gatewayname.appname.ip.nip.io
		defaultHostname := fmt.Sprintf("%s.%s.%s.nip.io", resource.ResourceName, resource.ApplicationName, publicIP)
		return &defaultHostname, nil
	}
}
