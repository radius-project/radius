// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
)

const (
	subscriptionID  = "default"
	resourceGroup   = "default"
	applicationName = "test-application"
	resourceName    = "test-gateway"
	publicIP        = "86.753.099.99"
)

func Test_GetDependencyIDs_Empty(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{}
	dependencies, _, err := r.GetDependencyIDs(context.Background(), resource)
	require.NoError(t, err)
	require.Empty(t, dependencies)
}

func Test_Render_WithNoHostname(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_WithPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "prefix"
	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{
			Prefix: &prefix,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", prefix, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_WithFQHostname(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "test-fqdn.contoso.com"
	expectedURL := "http://" + expectedHostname
	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{
			FullyQualifiedHostname: &expectedHostname,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_WithFQHostname_OverridesPrefix(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "http://test-fqdn.contoso.com"
	expectedURL := "http://" + expectedHostname
	prefix := "test-prefix"
	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{
			Prefix:                 &prefix,
			FullyQualifiedHostname: &expectedHostname,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_DevEnvironment(t *testing.T) {
	r := &Renderer{}

	publicIP := "http://localhost:32323"
	expectedFqdn := "localhost"
	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: true,
			PublicIP:               publicIP,
		},
	}

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, publicIP, output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, expectedFqdn, expectedIncludes)
}

func Test_Render_PublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	publicIP := "http://www.contoso.com:32323"
	expectedFqdn := "www.contoso.com"
	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: true,
			PublicIP:               publicIP,
		},
	}

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, publicIP, output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, expectedFqdn, expectedIncludes)
}

func Test_Render_WithMissingPublicIP(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(radclient.GatewayProperties{})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: false,
			PublicIP:               "",
		},
	}

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "unknown", output.ComputedValues["hostname"].Value)

	validateGateway(t, output.Resources, resource.ApplicationName, expectedIncludes)
}

func Test_Render_Fails_WithNoRoute(t *testing.T) {
	r := &Renderer{}

	properties := radclient.GatewayProperties{}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.Error(t, err)
	require.Equal(t, err.Error(), "must have at least one route when declaring a Gateway resource")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Fails_WithoutFQHostnameOrPrefix(t *testing.T) {
	r := &Renderer{}

	properties := radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.Error(t, err)
	require.Equal(t, err.Error(), "getting hostname failed with error: must provide either prefix or fullyQualifiedHostname if hostname is specified")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Single_Route(t *testing.T) {
	r := &Renderer{}

	var routes []*radclient.GatewayRoute
	routeName := "routename"
	destination := makeRouteResourceID(routeName)
	path := "/"
	route := radclient.GatewayRoute{
		Destination: &destination,
		Path:        &path,
	}
	routes = append(routes, &route)
	properties := radclient.GatewayProperties{
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["hostname"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.MakeResourceName(applicationName, routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: path,
				},
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
	validateHttpRoute(t, output.Resources, routeName, path)
}

func Test_Render_Multiple_Routes(t *testing.T) {
	r := &Renderer{}

	var routes []*radclient.GatewayRoute
	routeAName := "routename"
	routeADestination := makeRouteResourceID(routeAName)
	routeAPath := "/routea"
	routeA := radclient.GatewayRoute{
		Destination: &routeADestination,
		Path:        &routeAPath,
	}
	routeBName := "routename"
	routeBDestination := makeRouteResourceID(routeBName)
	routeBPath := "/routeb"
	routeB := radclient.GatewayRoute{
		Destination: &routeBDestination,
		Path:        &routeBPath,
	}
	routes = append(routes, &routeA)
	routes = append(routes, &routeB)
	properties := radclient.GatewayProperties{
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["hostname"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.MakeResourceName(applicationName, routeAName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeAPath,
				},
			},
		},
		{
			Name: kubernetes.MakeResourceName(applicationName, routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeBPath,
				},
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
	validateHttpRoute(t, output.Resources, routeAName, routeAPath)
	validateHttpRoute(t, output.Resources, routeBName, routeBPath)
}

func validateGateway(t *testing.T, outputResources []outputresource.OutputResource, expectedHostname string, expectedIncludes []contourv1.Include) {
	gateway, gatewayOutputResource := kubernetes.FindGateway(outputResources)

	expectedGatewayOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedGatewayOutputResource, gatewayOutputResource)
	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), gateway.Name)
	require.Equal(t, applicationName, gateway.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), gateway.Labels)

	var expectedVirtualHost *contourv1.VirtualHost = nil
	var expectedGatewaySpec contourv1.HTTPProxySpec
	if expectedHostname != "" {
		expectedVirtualHost = &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		}
		expectedGatewaySpec = contourv1.HTTPProxySpec{
			VirtualHost: expectedVirtualHost,
			Includes:    expectedIncludes,
		}
	} else {
		expectedGatewaySpec = contourv1.HTTPProxySpec{
			Includes: expectedIncludes,
		}
	}

	require.Equal(t, expectedVirtualHost, gateway.Spec.VirtualHost)
	require.Equal(t, expectedGatewaySpec, gateway.Spec)
}

func validateHttpRoute(t *testing.T, outputResources []outputresource.OutputResource, expectedRouteName, expectedMatchPath string) {
	expectedLocalID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, expectedRouteName)
	httpRoute, httpRouteOutputResource := kubernetes.FindHttpRouteByLocalID(outputResources, expectedLocalID)
	expectedHttpRouteOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, expectedLocalID, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedHttpRouteOutputResource, httpRouteOutputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, expectedRouteName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, expectedRouteName), httpRoute.Labels)

	require.Nil(t, httpRoute.Spec.VirtualHost)

	expectedPort := 80
	expectedServiceName := kubernetes.MakeResourceName(applicationName, expectedRouteName)

	expectedHttpRouteSpec := contourv1.HTTPProxySpec{
		Routes: []contourv1.Route{
			{
				Services: []contourv1.Service{
					{
						Name: expectedServiceName,
						Port: expectedPort,
					},
				},
			},
		},
	}

	require.Equal(t, expectedHttpRouteSpec, httpRoute.Spec)
}

func makeRouteResourceID(routeName string) string {
	return azresources.MakeID(
		subscriptionID,
		resourceGroup,
		azresources.ResourceType{
			Type: "Microsoft.CustomProviders",
			Name: "resourceProviders/radiusv3",
		},
		azresources.ResourceType{
			Type: "Application",
			Name: applicationName,
		},
		azresources.ResourceType{
			Type: "HttpRoute",
			Name: routeName,
		},
	)
}

func makeResource(t *testing.T, properties radclient.GatewayProperties) renderers.RendererResource {
	b, err := json.Marshal(&properties)
	require.NoError(t, err)

	definition := map[string]interface{}{}
	err = json.Unmarshal(b, &definition)
	require.NoError(t, err)

	return renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition:      definition,
	}
}

func makeTestGateway(config radclient.GatewayProperties) (radclient.GatewayProperties, []contourv1.Include) {
	routeName := "routeName"
	routeDestination := makeRouteResourceID("routeName")
	routePath := "/"
	defaultRoute := radclient.GatewayRoute{
		Destination: &routeDestination,
		Path:        &routePath,
	}

	includes := []contourv1.Include{
		{
			Name: kubernetes.MakeResourceName(applicationName, routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routePath,
				},
			},
		},
	}

	properties := radclient.GatewayProperties{
		Hostname: config.Hostname,
		Routes: []*radclient.GatewayRoute{
			&defaultRoute,
		},
	}

	return properties, includes
}

func GetRuntimeOptions() renderers.RuntimeOptions {
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			PublicIP: publicIP,
		},
	}
	return additionalProperties
}
