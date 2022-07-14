// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/ucp/resources"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-application"
	resourceName    = "test-gateway"
	publicIP        = "86.753.099.99"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_GetDependencyIDs_Success(t *testing.T) {
	testRouteAResourceID := makeRouteResourceID("testroutea")
	testRouteBResourceID := makeRouteResourceID("testrouteb")
	properties := datamodel.GatewayProperties{
		Routes: []datamodel.GatewayRoute{
			{
				Destination: testRouteAResourceID,
			},
			{
				Destination: testRouteBResourceID,
			},
		},
	}
	resource := makeResource(t, properties)

	renderer := Renderer{}
	radiusResourceIDs, resourceIDs, err := renderer.GetDependencyIDs(createContext(t), resource)
	require.NoError(t, err)
	require.Len(t, radiusResourceIDs, 2)
	require.Len(t, resourceIDs, 0)

	expectedRadiusResourceIDs := []resources.ID{
		makeResourceID(t, testRouteAResourceID),
		makeResourceID(t, testRouteBResourceID),
	}
	require.ElementsMatch(t, expectedRadiusResourceIDs, radiusResourceIDs)

	expectedAzureResourceIDs := []resources.ID{}
	require.ElementsMatch(t, expectedAzureResourceIDs, resourceIDs)
}

func Test_Render_WithNoHostname(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_WithPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "prefix"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", prefix, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_WithFQHostname(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "test-fqdn.contoso.com"
	expectedURL := "http://" + expectedHostname
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			FullyQualifiedHostname: expectedHostname,
		},
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_WithFQHostname_OverridesPrefix(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "http://test-fqdn.contoso.com"
	expectedURL := "http://" + expectedHostname
	prefix := "test-prefix"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix:                 prefix,
			FullyQualifiedHostname: expectedHostname,
		},
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
}

func Test_Render_DevEnvironment(t *testing.T) {
	r := &Renderer{}

	publicIP := "http://localhost:32323"
	expectedFqdn := "localhost"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := renderers.EnvironmentOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: true,
			PublicIP:               publicIP,
		},
		Namespace: applicationName,
	}

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, publicIP, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFqdn, expectedIncludes)
}

func Test_Render_PublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	publicIP := "http://www.contoso.com:32323"
	expectedFqdn := "www.contoso.com"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := renderers.EnvironmentOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: true,
			PublicIP:               publicIP,
		},
		Namespace: applicationName,
	}

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, publicIP, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFqdn, expectedIncludes)
}

func Test_Render_WithMissingPublicIP(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	})
	resource := makeResource(t, properties)
	appId, err := resources.Parse(resource.Properties.Application)
	require.NoError(t, err)
	appName := appId.Name()
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := renderers.EnvironmentOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: false,
			PublicIP:               "",
		},
		Namespace: applicationName,
	}

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "unknown", output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, appName, expectedIncludes)
}

func Test_Render_Fails_WithNoRoute(t *testing.T) {
	r := &Renderer{}

	properties := datamodel.GatewayProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.Error(), "must have at least one route when declaring a Gateway resource")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Fails_WithoutFQHostnameOrPrefix(t *testing.T) {
	r := &Renderer{}

	properties := datamodel.GatewayProperties{
		Hostname:    &datamodel.GatewayPropertiesHostname{},
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, "getting hostname failed with error: must provide either prefix or fullyQualifiedHostname if hostname is specified", err.Error())
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Single_Route(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeName := "routename"
	destination := makeRouteResourceID(routeName)
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: destination,
		Path:        path,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		Routes:      routes,
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

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
	validateHttpRoute(t, output.Resources, routeName, 80, nil)
}

func Test_Render_Multiple_Routes(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeAName := "routeaname"
	routeADestination := makeRouteResourceID(routeAName)
	routeAPath := "/routea"
	routeA := datamodel.GatewayRoute{
		Destination: routeADestination,
		Path:        routeAPath,
	}
	routeBName := "routenbname"
	routeBDestination := makeRouteResourceID(routeBName)
	routeBPath := "/routeb"
	routeB := datamodel.GatewayRoute{
		Destination: routeBDestination,
		Path:        routeBPath,
	}
	routes = append(routes, routeA)
	routes = append(routes, routeB)
	properties := datamodel.GatewayProperties{
		Routes:      routes,
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

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
	validateHttpRoute(t, output.Resources, routeAName, 80, nil)
	validateHttpRoute(t, output.Resources, routeBName, 80, nil)
}

func Test_Render_Route_WithPrefixRewrite(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeName := "routename"
	destination := makeRouteResourceID(routeName)
	path := "/backend"
	rewrite := "/rewrite"
	route := datamodel.GatewayRoute{
		Destination:   destination,
		Path:          path,
		ReplacePrefix: rewrite,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		Routes:      routes,
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

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

	expectedPathRewritePolicy := &contourv1.PathRewritePolicy{
		ReplacePrefix: []contourv1.ReplacePrefix{
			{
				Prefix:      path,
				Replacement: rewrite,
			},
		},
	}
	validateHttpRoute(t, output.Resources, routeName, 80, expectedPathRewritePolicy)
}

func Test_Render_Route_WithMultiplePrefixRewrite(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeAName := "routeaname"
	routeBName := "routebname"
	destinationA := makeRouteResourceID(routeAName)
	destinationB := makeRouteResourceID(routeBName)
	routeAPath := "/routea"
	routeA := datamodel.GatewayRoute{
		Destination: destinationA,
		Path:        routeAPath,
	}
	routeBPath := "/routeb"
	routeBRewrite := "routebrewrite"
	routeB := datamodel.GatewayRoute{
		Destination:   destinationB,
		Path:          routeBPath,
		ReplacePrefix: routeBRewrite,
	}
	routeCPath := "/routec"
	routeCRewrite := "routecrewrite"
	routeC := datamodel.GatewayRoute{
		Destination:   destinationB,
		Path:          routeCPath,
		ReplacePrefix: routeCRewrite,
	}
	routeDPath := "/routed"
	routeD := datamodel.GatewayRoute{
		Destination: destinationB,
		Path:        routeDPath,
	}
	routes = append(routes, routeA)
	routes = append(routes, routeB)
	routes = append(routes, routeC)
	routes = append(routes, routeD)
	properties := datamodel.GatewayProperties{
		Routes:      routes,
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

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
		{
			Name: kubernetes.MakeResourceName(applicationName, routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeCPath,
				},
			},
		},
		{
			Name: kubernetes.MakeResourceName(applicationName, routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeDPath,
				},
			},
		},
	}
	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)

	expectedPathRewritePolicy := &contourv1.PathRewritePolicy{
		ReplacePrefix: []contourv1.ReplacePrefix{
			{
				Prefix:      routeBPath,
				Replacement: routeBRewrite,
			},
			{
				Prefix:      routeCPath,
				Replacement: routeCRewrite,
			},
		},
	}
	validateHttpRoute(t, output.Resources, routeAName, 80, nil)
	validateHttpRoute(t, output.Resources, routeBName, 80, expectedPathRewritePolicy)
}

func Test_Render_WithDependencies(t *testing.T) {
	r := &Renderer{}

	var httpRoutePort int32 = 81
	httpRoute := renderHttpRoute(t, httpRoutePort)

	var routes []datamodel.GatewayRoute
	routeName := "routename"
	routeDestination := makeRouteResourceID(routeName)
	routePath := "/routea"
	port := float64((httpRoute.ComputedValues["port"].Value).(int32))

	route := datamodel.GatewayRoute{
		Destination: routeDestination,
		Path:        routePath,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		Routes:      routes,
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeResourceID(t, routeDestination).String()): {
			ResourceID: makeResourceID(t, routeDestination),
			Definition: map[string]interface{}{},
			ComputedValues: map[string]interface{}{
				"port": port,
			},
		},
	}

	environmentOptions := GetEnvironmentOptions()
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.MakeResourceName(applicationName, routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routePath,
				},
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes)
	validateHttpRoute(t, output.Resources, routeName, httpRoutePort, nil)
}

func renderHttpRoute(t *testing.T, port int32) renderers.RendererOutput {
	r := &httproute.Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	properties := datamodel.HTTPRouteProperties{
		Port:        port,
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
	}
	resource := makeDependentResource(t, properties)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{}})
	require.NoError(t, err)

	return output
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

func validateHttpRoute(t *testing.T, outputResources []outputresource.OutputResource, expectedRouteName string, expectedPort int32, expectedRewrite *contourv1.PathRewritePolicy) {
	expectedLocalID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, expectedRouteName)
	httpRoute, httpRouteOutputResource := kubernetes.FindHttpRouteByLocalID(outputResources, expectedLocalID)
	expectedHttpRouteOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, expectedLocalID, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedHttpRouteOutputResource, httpRouteOutputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, expectedRouteName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, expectedRouteName), httpRoute.Labels)

	require.Nil(t, httpRoute.Spec.VirtualHost)

	expectedServiceName := kubernetes.MakeResourceName(applicationName, expectedRouteName)

	expectedHttpRouteSpec := contourv1.HTTPProxySpec{
		Routes: []contourv1.Route{
			{
				Services: []contourv1.Service{
					{
						Name: expectedServiceName,
						Port: int(expectedPort),
					},
				},
				PathRewritePolicy: expectedRewrite,
			},
		},
	}

	require.Equal(t, expectedHttpRouteSpec, httpRoute.Spec)
}

func makeRouteResourceID(routeName string) string {

	return resources.MakeRelativeID(
		[]resources.ScopeSegment{
			{Type: "subscriptions", Name: "test-subscription"},
			{Type: "resourceGroups", Name: "test-resourcegroup"},
		},
		resources.TypeSegment{
			Type: "radius.dev/Application",
			Name: applicationName,
		},
		resources.TypeSegment{
			Type: "HttpRoute",
			Name: routeName,
		})
}

func makeResource(t *testing.T, properties datamodel.GatewayProperties) *datamodel.Gateway {
	return &datamodel.Gateway{
		TrackedResource: apiv1.TrackedResource{
			ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/gateways/test-gateway",
			Name: resourceName,
			Type: "Applications.Core/gateways",
		},
		Properties: properties,
	}
}
func makeDependentResource(t *testing.T, properties datamodel.HTTPRouteProperties) *datamodel.HTTPRoute {
	dm := datamodel.HTTPRoute{Properties: &properties}
	dm.Name = resourceName

	return &dm
}
func makeResourceID(t *testing.T, resourceID string) resources.ID {
	id, err := resources.Parse(resourceID)
	require.NoError(t, err)

	return id
}

func makeTestGateway(config datamodel.GatewayProperties) (datamodel.GatewayProperties, []contourv1.Include) {
	routeName := "routeName"
	routeDestination := makeRouteResourceID("routeName")
	routePath := "/"
	defaultRoute := datamodel.GatewayRoute{
		Destination: routeDestination,
		Path:        routePath,
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

	properties := datamodel.GatewayProperties{
		Hostname: config.Hostname,
		Routes: []datamodel.GatewayRoute{
			defaultRoute,
		},
		Application: config.Application,
	}

	return properties, includes
}

func GetEnvironmentOptions() renderers.EnvironmentOptions {
	environmentOptions := renderers.EnvironmentOptions{
		Gateway: renderers.GatewayOptions{
			PublicIP: publicIP,
		},
		Namespace: applicationName,
	}
	return environmentOptions
}
