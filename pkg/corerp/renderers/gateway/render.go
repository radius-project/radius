/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package gateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"strconv"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type Renderer struct {
}

// # Function Explanation
//
// GetDependencyIDs parses the gateway data model to get the resource IDs of the httpRoutes and the secretStore resource ID
// from the certificateFrom property, and returns them as two slices of resource IDs.
func (r Renderer) GetDependencyIDs(ctx context.Context, dm v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error) {
	gateway, ok := dm.(*datamodel.Gateway)
	if !ok {
		return nil, nil, v1.ErrInvalidModelConversion
	}
	gtwyProperties := gateway.Properties

	// Get all httpRoutes that are used by this gateway
	for _, route := range gtwyProperties.Routes {
		// Skip if destination is a URL. DNS-SD will resolve the route.
		if (isURL(route.Destination)) {
			continue
		}

		resourceID, err := resources.ParseResource(route.Destination)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	// Get secretStore resource ID from certificateFrom property
	if gtwyProperties.TLS != nil && gtwyProperties.TLS.CertificateFrom != "" {
		resourceID, err := resources.ParseResource(gtwyProperties.TLS.CertificateFrom)
		if err != nil {
			return nil, nil, v1.NewClientErrInvalidRequest(err.Error())
		}

		radiusResourceIDs = append(radiusResourceIDs, resourceID)
	}

	return radiusResourceIDs, azureResourceIDs, nil
}

// # Function Explanation
//
// Render creates a gateway object and http route objects based on the given parameters, and returns them along
// with a computed value for the gateway's public endpoint.
func (r Renderer) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	outputResources := []rpv1.OutputResource{}
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
		isHttps := gateway.Properties.TLS != nil && (gateway.Properties.TLS.SSLPassthrough || gateway.Properties.TLS.CertificateFrom != "")
		publicEndpoint = getPublicEndpoint(hostname, options.Environment.Gateway.Port, isHttps)
	}

	gatewayObject, err := MakeGateway(ctx, options, gateway, gateway.Name, applicationName, hostname)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	outputResources = append(outputResources, gatewayObject)

	computedValues := map[string]rpv1.ComputedValueReference{
		"url": {
			Value: publicEndpoint,
		},
	}

	httpRouteObjects, err := MakeHttpRoutes(ctx, options, *gateway, &gateway.Properties, gatewayName, applicationName)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	outputResources = append(outputResources, httpRouteObjects...)

	return renderers.RendererOutput{
		Resources:      outputResources,
		ComputedValues: computedValues,
	}, nil
}

// # Function Explanation
//
// MakeGateway validates the Gateway resource and its dependencies, and creates a Contour HTTPProxy resource
// to act as the Gateway.
func MakeGateway(ctx context.Context, options renderers.RenderOptions, gateway *datamodel.Gateway, resourceName string, applicationName string, hostname string) (rpv1.OutputResource, error) {
	includes := []contourv1.Include{}
	dependencies := options.Dependencies

	if len(gateway.Properties.Routes) < 1 {
		return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("must have at least one route when declaring a Gateway resource")
	}

	sslPassthrough := false
	var contourTLSConfig *contourv1.TLS

	// configure TLS if it is enabled
	if gateway.Properties.TLS != nil {
		sslPassthrough = gateway.Properties.TLS.SSLPassthrough

		if gateway.Properties.TLS.CertificateFrom != "" {
			secretStoreResourceId := gateway.Properties.TLS.CertificateFrom
			secretStoreResource, ok := dependencies[secretStoreResourceId]
			if !ok {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("secretStore resource %s not found", secretStoreResourceId))
			}

			referencedResource := dependencies[secretStoreResourceId].Resource
			if !strings.EqualFold(referencedResource.ResourceTypeName(), datamodel.SecretStoreResourceType) {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("certificateFrom must reference a secretStore resource")
			}

			// Validate the secretStore resource: it must be of type certificate and have tls.crt and tls.key
			secretStore, ok := referencedResource.(*datamodel.SecretStore)
			if !ok {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("certificateFrom must reference a secretStore resource")
			}

			if secretStore.Properties.Type != datamodel.SecretTypeCert {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("certificateFrom must reference a secretStore resource with type certificate")
			}

			if secretStore.Properties.Data["tls.crt"] == nil {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("certificateFrom must reference a secretStore resource with tls.crt")
			}

			if secretStore.Properties.Data["tls.key"] == nil {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("certificateFrom must reference a secretStore resource with tls.key")
			}

			// Get the name and namespace of the Kubernetes secret resource from the secretStore OutputResources
			if secretStoreResource.OutputResources == nil {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("secretStore resource %s not found", secretStoreResourceId))
			}

			secretResource := secretStoreResource.OutputResources[rpv1.LocalIDSecret].Data
			secretResourceData, ok := secretResource.(map[string]any)
			if !ok {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("secretStore resource %s not found", secretStoreResourceId))
			}

			secretName, ok := secretResourceData["name"]
			if !ok {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("secretStore resource %s not found", secretStoreResourceId))
			}

			secretNamespace, ok := secretResourceData["namespace"]
			if !ok {
				return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("secretStore resource %s not found", secretStoreResourceId))
			}

			contourTLSConfig = &contourv1.TLS{
				SecretName:             fmt.Sprintf("%s/%s", secretNamespace, secretName),
				MinimumProtocolVersion: string(gateway.Properties.TLS.MinimumProtocolVersion),
			}
		}
	}

	// If SSL Passthrough is enabled, then we can only have one route
	if sslPassthrough && len(gateway.Properties.Routes) > 1 {
		return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("cannot support multiple routes with sslPassthrough set to true")
	}

	var route datamodel.GatewayRoute //route will hold the one sslPassthrough route, if sslPassthrough is true
	for _, route = range gateway.Properties.Routes {
		if sslPassthrough && (route.Path != "" || route.ReplacePrefix != "") {
			return rpv1.OutputResource{}, v1.NewClientErrInvalidRequest("cannot support `path` or `replacePrefix` in routes with sslPassthrough set to true")
		}
		routeName, err := getRouteName(&route)
		if err != nil {
			return rpv1.OutputResource{}, err
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
		TLS:  contourTLSConfig,
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
			return rpv1.OutputResource{}, err
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
			Name:        kubernetes.NormalizeResourceName(resourceName),
			Namespace:   options.Environment.Namespace,
			Labels:      renderers.GetLabels(ctx, options, applicationName, resourceName, gateway.ResourceTypeName()),
			Annotations: renderers.GetAnnotations(ctx, options),
		},
		Spec: contourv1.HTTPProxySpec{
			VirtualHost: virtualHost,
			Includes:    includes,
		},
	}
	if sslPassthrough {
		rootHTTPProxy.Spec.TCPProxy = tcpProxy
	}

	return rpv1.NewKubernetesOutputResource(resourcekinds.Gateway, rpv1.LocalIDGateway, rootHTTPProxy, rootHTTPProxy.ObjectMeta), nil
}

// # Function Explanation
//
// MakeHttpRoutes creates HTTPProxy objects for each route in the gateway and returns them as OutputResources. It returns
// an error if it fails to get the route name.
func MakeHttpRoutes(ctx context.Context, options renderers.RenderOptions, resource datamodel.Gateway, gateway *datamodel.GatewayProperties, gatewayName string, applicationName string) ([]rpv1.OutputResource, error) {
	dependencies := options.Dependencies
	objects := make(map[string]*contourv1.HTTPProxy)

	for _, route := range gateway.Routes {
		routeProperties := dependencies[route.Destination]
		port := renderers.DefaultPort
		routePort, ok := routeProperties.ComputedValues["port"].(float64)
		if ok {
			port = int32(routePort)
		}

		// if the route destination is a URL, then we need to parse the port from the URL
		if isURL(route.Destination) {
			_, _, urlPort, err := parseURL(route.Destination)
			if err != nil {
				return []rpv1.OutputResource{}, err
			}

			intURLport, err := strconv.Atoi(urlPort)
			if err != nil {
				return []rpv1.OutputResource{}, err
			}

			// bound check intURLport
			if intURLport < 0 || intURLport > 65535 {
				return []rpv1.OutputResource{}, fmt.Errorf("port %d is out of range", intURLport)
			}
			
			port = int32(intURLport)
		}

		routeName, err := getRouteName(&route)
		if err != nil {
			return []rpv1.OutputResource{}, err
		}

		// Create unique localID for dependency graph
		localID := fmt.Sprintf("%s-%s", rpv1.LocalIDHttpRoute, routeName)
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
				Name:        routeResourceName,
				Namespace:   options.Environment.Namespace,
				Labels:      renderers.GetLabels(ctx, options, applicationName, routeName, resource.ResourceTypeName()),
				Annotations: renderers.GetAnnotations(ctx, options),
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

	var outputResources []rpv1.OutputResource
	for localID, object := range objects {
		outputResources = append(outputResources, rpv1.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, localID, object, object.ObjectMeta))
	}

	return outputResources, nil
}

func getRouteName(route *datamodel.GatewayRoute) (string, error) {
	// if isURL, then name is hostname (DNS-SD case)
	if isURL(route.Destination) {
		u, err := url.Parse(route.Destination)
		if err != nil {
			return "", v1.NewClientErrInvalidRequest(err.Error())
		}

		return u.Hostname(), nil
	}

	// if not URL, then name is the resourceID (HTTProute case)
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

// getPublicEndpoint adds http:// or https:// and the port (if it exists) to the given hostname
func getPublicEndpoint(hostname string, port string, isHttps bool) string {
	authority := hostname
	if port != "" {
		authority = net.JoinHostPort(hostname, port)
	}

	scheme := "http"
	if isHttps {
		scheme = "https"
	}

	return fmt.Sprintf("%s://%s", scheme, authority)
}

func isURL(input string) bool {
	_, err := url.ParseRequestURI(input)
	fmt.Println(err)
	fmt.Println(input)
	// if first character is a slash, it's not a URL. It's a path.
	if (err != nil || input[0] == '/') {
		return false
	}
	return true
}

func parseURL(sourceURL string) (scheme, hostname, port string, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", "", err
	}

	scheme = u.Scheme
	host := u.Host

	hostname, port, err = net.SplitHostPort(host)
	if err != nil {
		return "", "", "", err
	}

	return scheme, hostname, port, nil
}

