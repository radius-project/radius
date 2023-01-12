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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
)

type Renderer struct {
}

// GetDependencyIDs fetches all the httproutes used by the gateway
func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	// Need all httproutes that are used by this gateway
	gateway, ok := dm.(*datamodel.Gateway)
	if !ok {
		return nil, nil, v1.ErrInvalidModelConversion
	}
	gtwyProperties := gateway.Properties
	// Get all httproutes that are used by this gateway
	for _, httpRoute := range gtwyProperties.Routes {
		resourceID, err := resources.ParseResource(httpRoute.Destination)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// Render creates the kubernetes output resource for the gateway and its dependency - httproute
func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []outputresource.OutputResource{}
	gateway, ok := dm.(*datamodel.Gateway)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}
	appId, err := resources.ParseResource(gateway.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("invalid application id: %s. id: %s", err.Error(), gateway.Properties.Application))
	}
	applicationName := appId.Name()
	gatewayName := kubernetes.NormalizeResourceName(gateway.Name)
	hostname, err := getHostname(*gateway, &gateway.Properties, applicationName, options.Environment.Gateway)

	var publicEndpoint string
	if errors.Is(err, &ErrNoPublicEndpoint{}) {
		publicEndpoint = "unknown"
	} else if err != nil {
		return renderers.RendererOutput{}, fmt.Errorf("getting hostname failed with error: %s", err)
	} else {
		publicEndpoint = getPublicEndpoint(hostname, options.Environment.Gateway.Port)
	}

	gatewayObject, err := MakeGateway(options, gateway, gateway.Name, applicationName, hostname)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResources = append(outputResources, gatewayObject)

	computedValues := map[string]rp.ComputedValueReference{
		"url": {
			Value: publicEndpoint,
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
	sslPassthrough := false

	if len(gateway.Properties.Routes) < 1 {
		return outputresource.OutputResource{}, v1.NewClientErrInvalidRequest("must have at least one route when declaring a Gateway resource")
	}

	if gateway.Properties.TLS != nil {
		if !gateway.Properties.TLS.SSLPassthrough {
			return outputresource.OutputResource{}, v1.NewClientErrInvalidRequest("only sslPassthrough is supported for TLS currently")
		} else {
			sslPassthrough = true
		}
	}

	if sslPassthrough && len(gateway.Properties.Routes) > 1 {
		return outputresource.OutputResource{}, v1.NewClientErrInvalidRequest("cannot support multiple routes with sslPassthrough set to true")
	}

	var route datamodel.GatewayRoute //route will hold the one sslPassthrough route, if sslPassthrough is true
	for _, route = range gateway.Properties.Routes {
		if sslPassthrough && (route.Path != "" || route.ReplacePrefix != "") {
			return outputresource.OutputResource{}, v1.NewClientErrInvalidRequest("cannot support `path` or `replacePrefix` in routes with sslPassthrough set to true")
		}
		routeName, err := getRouteName(&route)
		if err != nil {
			return outputresource.OutputResource{}, err
		}

		routeResourceName := kubernetes.NormalizeResourceName(routeName)
		prefix := route.Path
		if sslPassthrough {
			prefix = "/"
		}
		includes = append(includes, contourv1.Include{
			Name: routeResourceName,
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: prefix,
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

	var tcpProxy *contourv1.TCPProxy
	if sslPassthrough {
		virtualHost.TLS = &contourv1.TLS{
			Passthrough: true,
		}

		routeProperties := options.Dependencies[route.Destination]
		port := renderers.DefaultSecurePort
		routePort, ok := routeProperties.ComputedValues["port"].(float64)
		if ok {
			port = int32(routePort)
		}

		routeName, err := getRouteName(&route)
		if err != nil {
			return outputresource.OutputResource{}, err
		}

		tcpProxy = &contourv1.TCPProxy{
			Services: []contourv1.Service{
				{
					Name: kubernetes.NormalizeResourceName(routeName),
					Port: int(port),
				},
			},
		}
	}

	// The root HTTPProxy object acts as the Gateway
	rootHTTPProxy := &contourv1.HTTPProxy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPProxy",
			APIVersion: contourv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      kubernetes.NormalizeResourceName(resourceName),
			Namespace: options.Environment.Namespace,
			Labels:    kubernetes.MakeDescriptiveLabels(applicationName, resourceName, gateway.ResourceTypeName()),
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: virtualHost,
			Includes:    includes,
		},
	}
	if sslPassthrough {
		rootHTTPProxy.Spec.TCPProxy = tcpProxy
	}

	return outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, rootHTTPProxy, rootHTTPProxy.ObjectMeta), nil
}

// MakeHttpRoutes creates the kubernetes httproute construct from the corerp gateway datamodel
func MakeHttpRoutes(options renderers.RenderOptions, resource datamodel.Gateway, gateway *datamodel.GatewayProperties, gatewayName string, applicationName string) ([]outputresource.OutputResource, error) {
	dependencies := options.Dependencies
	objects := make(map[string]*contourv1.HTTPProxy)

	for _, route := range gateway.Routes {
		routeProperties := dependencies[route.Destination]
		port := renderers.DefaultPort
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
		routeResourceName := kubernetes.NormalizeResourceName(routeName)

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
				Labels:    kubernetes.MakeDescriptiveLabels(applicationName, routeName, resource.ResourceTypeName()),
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
	resourceID, err := resources.ParseResource(route.Destination)
	if err != nil {
		return "", v1.NewClientErrInvalidRequest(err.Error())
	}

	return resourceID.Name(), nil
}

// getHostname returns the hostname of the public endpoint of the Gateway.
// This sometimes involves transforming the external IP of the cluster into
// a hostname that's unique to this Gateway and Application.
func getHostname(resource datamodel.Gateway, gateway *datamodel.GatewayProperties, applicationName string, options renderers.GatewayOptions) (string, error) {
	// Handle the explicit override case (return)
	// Handle the explicit FQDN case (return)
	// Select the 'base' hostname (convert IP to hostname)
	// If a prefix is not specified and the LoadBalancer provided an IP, then prepend with the gateway name
	// Prepend the prefix, if one is specified
	// Return the (possibly altered) hostname

	if options.PublicEndpointOverride {
		// Specified from --public-endpoint-override CLI flag
		return options.Hostname, nil
	} else if gateway.Hostname != nil && gateway.Hostname.FullyQualifiedHostname != "" {
		// Trust that the provided FullyQualifiedHostname actually works
		return gateway.Hostname.FullyQualifiedHostname, nil
	}

	// baseHostname represents the base hostname that may be prepended with the given prefix
	// or gateway name. After this block, baseHostname looks like either:
	// 1. a hostname from the LoadBalancer
	// 2. appname.IP.nip.io
	var baseHostname string
	if options.ExternalIP != "" {
		baseHostname = fmt.Sprintf("%s.%s.nip.io", applicationName, options.ExternalIP)

		// If no prefix was specified, and the LoadBalancer provided us an ExternalIP,
		// prepend the hostname with the Gateway name (for uniqueness)
		if gateway.Hostname == nil {
			// Auto-assign hostname: gatewayname.appname.ip.nip.io
			return fmt.Sprintf("%s.%s", resource.Name, baseHostname), nil
		}
	} else if options.Hostname != "" {
		baseHostname = options.Hostname
	} else {
		// In the case of no public endpoint, use the application name as the hostname
		return applicationName, &ErrNoPublicEndpoint{}
	}

	// Prepend the prefix, if the user specified one
	if gateway.Hostname != nil {
		// Generate a hostname using the external IP
		if gateway.Hostname.Prefix != "" {
			// Auto-assign hostname: prefix.appname.ip.nip.io
			return fmt.Sprintf("%s.%s", gateway.Hostname.Prefix, baseHostname), nil
		} else {
			return "", &ErrFQDNOrPrefixRequired{}
		}
	}

	return baseHostname, nil
}

// getPublicEndpoint adds http:// and the port (if it exists) to the given hostname
func getPublicEndpoint(hostname string, port string) string {
	authority := hostname
	if port != "" {
		authority = net.JoinHostPort(hostname, port)
	}

	return fmt.Sprintf("http://%s", authority)
}
