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

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

type Renderer struct {
}

// GetDependencyIDs fetches all the httproutes used by the gateway
func (r Renderer) GetDependencyIDs(ctx context.Context, dm conv.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	// Need all httproutes that are used by this gateway
	gateway, ok := dm.(*datamodel.Gateway)
	if !ok {
		return nil, nil, conv.ErrInvalidModelConversion
	}
	gtwyProperties := gateway.Properties
	// Get all httproutes that are used by this gateway
	for _, httpRoute := range gtwyProperties.Routes {
		resourceID, err := resources.Parse(httpRoute.Destination)
		if err != nil {
			return nil, nil, err
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// Render creates the kubernetes output resource for the gateway and its dependency - httproute
func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []outputresource.OutputResource{}
	gateway, ok := dm.(*datamodel.Gateway)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}
	appId, err := resources.Parse(gateway.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("invalid application id: %w. id: %s", err, gateway.Properties.Application)
	}
	applicationName := appId.Name()
	gatewayName := kubernetes.MakeResourceName(applicationName, gateway.Name)
	hostname, err := getHostname(*gateway, &gateway.Properties, applicationName, options)
	if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("getting hostname failed with error: %s", err)
	}
	gatewayObject, err := MakeGateway(options, gateway, gateway.Name, applicationName, hostname)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResources = append(outputResources, gatewayObject)

	var computedHostname string
	if hostname == "" {
		computedHostname = "unknown"
	} else if options.Environment.Gateway.PublicEndpointOverride {
		computedHostname = options.Environment.Gateway.PublicIP
	} else {
		computedHostname = "http://" + hostname
	}

	computedValues := map[string]rp.ComputedValueReference{
		"url": {
			Value: computedHostname,
		},
	}

	httpRouteObjects, err := MakeHttpRoutes(options, *gateway, &gateway.Properties, gatewayName, applicationName)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, httpRouteObjects...)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

// MakeGateway creates the kubernetes gateway construct from the gateway corerp datamodel
func MakeGateway(options renderers.RenderOptions, gateway *datamodel.Gateway, resourceName string, applicationName string, hostname string) (outputresource.OutputResource, error) {
	includes := []contourv1.Include{}

	if len(gateway.Properties.Routes) < 1 {
		return outputresource.OutputResource{}, errors.New("must have at least one route when declaring a Gateway resource")
	}

	for _, route := range gateway.Properties.Routes {
		routeName, err := getRouteName(&route)
		if err != nil {
			return outputresource.OutputResource{}, err
		}

		routeResourceName := kubernetes.MakeResourceName(applicationName, routeName)

		includes = append(includes, contourv1.Include{
			Name: routeResourceName,
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: route.Path,
				},
			},
		})
	}

	virtualHostname := hostname
	if hostname == "" {
		// If the given hostname is empty, use the application name
		// in order to make sure that this resource is seen as a root proxy.
		virtualHostname = applicationName
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
			Name:      kubernetes.MakeResourceName(applicationName, resourceName),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resourceName),
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: virtualHost,
			Includes:    includes,
		},
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, rootHTTPProxy, rootHTTPProxy.ObjectMeta), nil
}

// MakeHttpRoutes creates the kubernetes httproute construct from the corerp gateway datamodel
func MakeHttpRoutes(options renderers.RenderOptions, resource datamodel.Gateway, gateway *datamodel.GatewayProperties, gatewayName string, applicationName string) ([]outputresource.OutputResource, error) {
	dependencies := options.Dependencies
	objects := make(map[string]*contourv1.HTTPProxy)

	for _, route := range gateway.Routes {
		routeProperties := dependencies[route.Destination]
		port := kubernetes.GetDefaultPort()

		// HACK, IDK why this returns a float64 instead of int32 when coming from the corerp
		routePort, ok := routeProperties.ComputedValues["port"].(float64)
		if ok {
			port = int32(routePort)
		}

		routeName, err := getRouteName(&route)
		if err != nil {
			return []outputresource.OutputResource{}, err
		}

		// Create unique localID for dependency graph
		localID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, routeName)
		routeResourceName := kubernetes.MakeResourceName(applicationName, routeName)

		var pathRewritePolicy *contourv1.PathRewritePolicy
		if route.ReplacePrefix != "" {
			pathRewritePolicy = &contourv1.PathRewritePolicy{
				ReplacePrefix: []contourv1.ReplacePrefix{
					{
						Prefix:      route.Path,
						Replacement: route.ReplacePrefix,
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
				Namespace: options.Environment.Namespace,
				Labels:    kubernetes.MakeDescriptiveLabels(applicationName, routeName),
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

func getRouteName(route *datamodel.GatewayRoute) (string, error) {
	resourceID, err := resources.Parse(route.Destination)
	if err != nil {
		return "", err
	}

	return resourceID.Name(), nil
}

func getHostname(resource datamodel.Gateway, gateway *datamodel.GatewayProperties, applicationName string, options renderers.RenderOptions) (string, error) {
	publicIP := options.Environment.Gateway.PublicIP
	publicEndpointOverride := options.Environment.Gateway.PublicEndpointOverride

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
		if gateway.Hostname.FullyQualifiedHostname != "" {
			// Use FQDN
			return gateway.Hostname.FullyQualifiedHostname, nil
		} else if gateway.Hostname.Prefix != "" {
			// Auto-assign hostname: prefix.appname.ip.nip.io
			prefixedHostname := fmt.Sprintf("%s.%s.%s.nip.io", gateway.Hostname.Prefix, applicationName, publicIP)
			return prefixedHostname, nil
		} else {
			return "", fmt.Errorf("must provide either prefix or fullyQualifiedHostname if hostname is specified")
		}
	} else {
		// Auto-assign hostname: gatewayname.appname.ip.nip.io
		defaultHostname := fmt.Sprintf("%s.%s.%s.nip.io", resource.Name, applicationName, publicIP)
		return defaultHostname, nil
	}
}
