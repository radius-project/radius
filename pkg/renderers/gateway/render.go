// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

type Renderer struct {
}

func (r Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) (radiusResourceIDs []azresources.ResourceID, azureResourceIDs []azresources.ResourceID, err error) {
	// Need all httproutes that are used by this gateway
	gateway, err := r.convert(resource)
	if err != nil {
		return nil, nil, err
	}

	for _, httpRoute := range gateway.Routes {
		resourceID, err := azresources.Parse(*httpRoute.Destination)
		if err != nil {
			return nil, nil, err
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

func (r Renderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []outputresource.OutputResource{}

	gateway, err := r.convert(options.Resource)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	gatewayName := kubernetes.MakeResourceName(options.Resource.ApplicationName, options.Resource.ResourceName)
	hostname, err := getHostname(options.Resource, gateway, options.Runtime)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("getting hostname failed with error: %s", err)
	}

	gatewayObject, err := MakeGateway(options, gateway, gatewayName, hostname)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResources = append(outputResources, gatewayObject)

	var computedHostname string
	if hostname == "" {
		computedHostname = "unknown"
	} else if options.Runtime.Gateway.PublicEndpointOverride {
		computedHostname = options.Runtime.Gateway.PublicIP
	} else {
		computedHostname = "http://" + hostname
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"url": {
			Value: computedHostname,
		},
	}

	httpRouteObjects, err := MakeHttpRoutes(options, gateway, gatewayName)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, httpRouteObjects...)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

func MakeGateway(options renderers.RenderOptions, gateway *radclient.GatewayProperties, gatewayName string, hostname string) (outputresource.OutputResource, error) {
	includes := []contourv1.Include{}

	if len(gateway.Routes) < 1 {
		return outputresource.OutputResource{}, errors.New("must have at least one route when declaring a Gateway resource")
	}

	for _, route := range gateway.Routes {
		routeName, err := getRouteName(route)
		if err != nil {
			return outputresource.OutputResource{}, err
		}

		routeResourceName := kubernetes.MakeResourceName(options.Resource.ApplicationName, routeName)

		includes = append(includes, contourv1.Include{
			Name: routeResourceName,
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: *route.Path,
				},
			},
		})
	}

	virtualHostname := hostname
	if hostname == "" {
		// If the given hostname is empty, use the application name
		// in order to make sure that this resource is seen as a root proxy.
		virtualHostname = options.Resource.ApplicationName
	}

	virtualHost := &contourv1.VirtualHost{
		Fqdn: virtualHostname,
	}

	// The root HTTPProxy object acts as the Gateway
	rootHTTPProxy := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: contourv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayName,
			Namespace: options.Resource.ApplicationName,
			Labels:    kubernetes.MakeDescriptiveLabels(options.Resource.ApplicationName, options.Resource.ResourceName),
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: virtualHost,
			Includes:    includes,
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, rootHTTPProxy, rootHTTPProxy.ObjectMeta), nil
}

func MakeHttpRoutes(options renderers.RenderOptions, gateway *radclient.GatewayProperties, gatewayName string) ([]outputresource.OutputResource, error) {
	resource := options.Resource
	dependencies := options.Dependencies
	objects := make(map[string]*contourv1.HTTPProxy)

	for _, route := range gateway.Routes {
		routeProperties := dependencies[to.String(route.Destination)]
		port := kubernetes.GetDefaultPort()
		routePort, ok := routeProperties.ComputedValues["port"].(int32)
		if ok {
			port = routePort
		}

		routeName, err := getRouteName(route)
		if err != nil {
			return []outputresource.OutputResource{}, err
		}

		// Create unique localID for dependency graph
		localID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, routeName)
		routeResourceName := kubernetes.MakeResourceName(resource.ApplicationName, routeName)

		var pathRewritePolicy *contourv1.PathRewritePolicy
		if route.ReplacePrefix != nil {
			pathRewritePolicy = &contourv1.PathRewritePolicy{
				ReplacePrefix: []contourv1.ReplacePrefix{
					{
						Prefix:      *route.Path,
						Replacement: *route.ReplacePrefix,
					},
				},
			}
		}

		// If this route already exists, append to it
		if object, exists := objects[localID]; exists {
			if pathRewritePolicy != nil {
			outer:
				for i := range object.Spec.Routes {
					for _, service := range object.Spec.Routes[i].Services {
						if service.Name == routeResourceName {
							if object.Spec.Routes[i].PathRewritePolicy == nil {
								object.Spec.Routes[i].PathRewritePolicy = pathRewritePolicy
							} else {
								object.Spec.Routes[i].PathRewritePolicy.ReplacePrefix = append(object.Spec.Routes[i].PathRewritePolicy.ReplacePrefix, pathRewritePolicy.ReplacePrefix[0])
							}

							break outer
						}
					}
				}
			}

			continue
		}

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
								Port: int(port),
							},
						},
						PathRewritePolicy: pathRewritePolicy,
					},
				},
			},
		}

		objects[localID] = httpProxyObject
	}

	var outputResources []outputresource.OutputResource
	for localID, object := range objects {
		outputResources = append(outputResources, outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, localID, object, object.ObjectMeta))
	}

	return outputResources, nil
}

func (r Renderer) convert(resource renderers.RendererResource) (*radclient.GatewayProperties, error) {
	properties := &radclient.GatewayProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func getRouteName(route *radclient.GatewayRoute) (string, error) {
	resourceID, err := azresources.Parse(*route.Destination)
	if err != nil {
		return "", err
	}

	return resourceID.Name(), nil
}

func getHostname(resource renderers.RendererResource, gateway *radclient.GatewayProperties, options renderers.RuntimeOptions) (string, error) {
	publicIP := options.Gateway.PublicIP
	publicEndpointOverride := options.Gateway.PublicEndpointOverride

	if publicEndpointOverride {
		// Local Dev scenario
		urlOverride, err := url.Parse(publicIP)
		if err != nil {
			return "", fmt.Errorf("unable to parse given url: %s", publicIP)
		}

		host, _, err := net.SplitHostPort(urlOverride.Host)
		if err != nil {
			return "", fmt.Errorf("unable to split host and port from given url: %s", urlOverride.Host)
		}

		return host, nil
	} else if publicIP == "" {
		// In the case of no publicIP, return an empty hostname, but don't return an error
		// Should be improved in https://github.com/project-radius/radius/issues/2196
		return "", nil
	} else if gateway.Hostname != nil {
		if gateway.Hostname.FullyQualifiedHostname != nil {
			// Use FQDN
			return *gateway.Hostname.FullyQualifiedHostname, nil
		} else if gateway.Hostname.Prefix != nil {
			// Auto-assign hostname: prefix.appname.ip.nip.io
			prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", *gateway.Hostname.Prefix, resource.ApplicationName, publicIP)
			return prefixedHostname, nil
		} else {
			return "", fmt.Errorf("must provide either prefix or fullyQualifiedHostname if hostname is specified")
		}
	} else {
		// Auto-assign hostname: gatewayname.appname.ip.nip.io
		defaultHostname := fmt.Sprintf("%s.%s.%s.nip.io", resource.ResourceName, resource.ApplicationName, publicIP)
		return defaultHostname, nil
	}
}
