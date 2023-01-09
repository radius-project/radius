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
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-application"
	resourceName    = "test-gateway"
	testExternalIP  = "86.753.099.99"
	testHostname    = "a3cce48e78bc14ae6b0be72e4a33a6e3-797173506.us-west-2.elb.amazonaws.com"
	testPort        = "8080"
)

func createContext(t *testing.T) context.Context {
	logger, err := ucplog.NewTestLogger(t)
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

func Test_Render_WithIPAndNoHostname(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
}

func Test_Render_WithIPAndPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "prefix"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", prefix, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
}

func Test_Render_WithIPAndFQHostname(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "test-fqdn.contoso.com"
	expectedPublicEndpoint := "http://test-fqdn.contoso.com"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			FullyQualifiedHostname: expectedHostname,
		},
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
}

func Test_Render_WithFQHostname_OverridesPrefix(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "test-fqdn.contoso.com"
	expectedPublicEndpoint := "http://test-fqdn.contoso.com"
	prefix := "test-prefix"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix:                 prefix,
			FullyQualifiedHostname: expectedHostname,
		},
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
}

func Test_Render_PublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(testHostname, "", testPort, true)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "http://"+testHostname+":"+testPort, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, testHostname, expectedIncludes, nil)
}

func Test_Render_PublicEndpointOverride_OverridesAll(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := "this_CouldbeAnyString"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix:                 "test",
			FullyQualifiedHostname: "testagain",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(expectedPublicEndpoint, testExternalIP, "", true)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "http://"+expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedPublicEndpoint, expectedIncludes, nil)
}

func Test_Render_PublicEndpointOverride_WithEmptyIP(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := "www.contoso.com"
	expectedFQDN := "www.contoso.com"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(expectedFQDN, "", "", true)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "http://"+expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFQDN, expectedIncludes, nil)
}

func Test_Render_LocalhostPublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	expectedFQDN := "localhost"
	expectedPublicEndpoint := "http://localhost:8080"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(expectedFQDN, "", testPort, true)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFQDN, expectedIncludes, nil)
}

func Test_Render_Hostname(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := fmt.Sprintf("http://%s", testHostname)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(testHostname, "", "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, testHostname, expectedIncludes, nil)
}

func Test_Render_Hostname_WithPort(t *testing.T) {
	r := &Renderer{}

	expectedFQDN := "www.contoso.com"
	expectedPublicEndpoint := "http://www.contoso.com:32434"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(expectedFQDN, "", "32434", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFQDN, expectedIncludes, nil)
}

func Test_Render_Hostname_WithPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "test"
	expectedFQDN := fmt.Sprintf("%s.%s", prefix, testHostname)
	expectedPublicEndpoint := fmt.Sprintf("http://%s", expectedFQDN)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(testHostname, "", "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFQDN, expectedIncludes, nil)
}

func Test_Render_Hostname_WithPrefixAndPort(t *testing.T) {
	r := &Renderer{}

	prefix := "test"
	expectedFQDN := fmt.Sprintf("%s.%s", prefix, testHostname)
	expectedPublicEndpoint := fmt.Sprintf("http://%s:%s", expectedFQDN, testPort)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions(testHostname, "", testPort, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, expectedFQDN, expectedIncludes, nil)
}

func Test_Render_WithMissingPublicIP(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	appId, err := resources.ParseResource(resource.Properties.Application)
	require.NoError(t, err)
	appName := appId.Name()
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", "", "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "unknown", output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, appName, expectedIncludes, nil)
}

func Test_Render_Fails_SSLPassthroughWithRoutePath(t *testing.T) {
	var routes []datamodel.GatewayRoute
	routeName := "routename"
	destination := makeRouteResourceID(routeName)
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: destination,
		Path:        path,
	}
	routes = append(routes, route)
	r := &Renderer{}
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			SSLPassthrough: true,
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*conv.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*conv.ErrClientRP).Message, "cannot support `path` or `replacePrefix` in routes with sslPassthrough set to true")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Fails_SSLPassthroughWithMultipleRoutes(t *testing.T) {
	var routes []datamodel.GatewayRoute
	routeName1 := "routename1"
	destination1 := makeRouteResourceID(routeName1)
	path := "/"
	route1 := datamodel.GatewayRoute{
		Destination: destination1,
		Path:        path,
	}
	routeName2 := "routename2"
	destination2 := makeRouteResourceID(routeName2)
	route2 := datamodel.GatewayRoute{
		Destination: destination2,
		Path:        path,
	}
	routes = append(routes, route1)
	routes = append(routes, route2)

	r := &Renderer{}
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			SSLPassthrough: true,
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*conv.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*conv.ErrClientRP).Message, "cannot support multiple routes with sslPassthrough set to true")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Fails_SSLPassthroughFalse(t *testing.T) {
	var routes []datamodel.GatewayRoute
	routeName := "routename1"
	destination := makeRouteResourceID(routeName)
	route1 := datamodel.GatewayRoute{
		Destination: destination,
	}
	routes = append(routes, route1)
	r := &Renderer{}
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			SSLPassthrough: false,
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*conv.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*conv.ErrClientRP).Message, "only sslPassthrough is supported for TLS currently")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Fails_WithNoRoute(t *testing.T) {
	r := &Renderer{}

	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*conv.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*conv.ErrClientRP).Message, "must have at least one route when declaring a Gateway resource")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_FQDNOverride(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := fmt.Sprintf("http://%s", testHostname)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			FullyQualifiedHostname: testHostname,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	validateGateway(t, output.Resources, testHostname, expectedIncludes, nil)
}

func Test_Render_Fails_WithoutFQHostnameOrPrefix(t *testing.T) {
	r := &Renderer{}

	properties, _ := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)

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
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName(routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: path,
				},
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
	validateHttpRoute(t, output.Resources, routeName, 80, nil)
}

func Test_Render_SSLPassthrough(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeName := "routename"
	destination := makeRouteResourceID(routeName)
	route := datamodel.GatewayRoute{
		Destination: destination,
	}
	routes = append(routes, route)
	tls := &datamodel.GatewayPropertiesTLS{
		SSLPassthrough: true,
	}
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
		TLS:    tls,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName(routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: "/",
				},
			},
		},
	}

	routeName, err = getRouteName(&route)
	require.NoError(t, err)

	// Create unique localID for dependency graph
	routeResourceName := kubernetes.NormalizeResourceName(routeName)

	expectedTCPProxy := &contourv1.TCPProxy{
		Services: []contourv1.Service{
			{
				Name: routeResourceName,
				Port: 443,
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, expectedTCPProxy)
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
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName(routeAName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeAPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName(routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeBPath,
				},
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
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
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName(routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: path,
				},
			},
		},
	}
	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)

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
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName(routeAName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeAPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName(routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeBPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName(routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeCPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName(routeBName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeDPath,
				},
			},
		},
	}
	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)

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
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{
		(makeResourceID(t, routeDestination).String()): {
			ResourceID: makeResourceID(t, routeDestination),
			ComputedValues: map[string]any{
				"port": port,
			},
		},
	}

	environmentOptions := GetEnvironmentOptions("", testExternalIP, "", false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedIncludes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName(routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routePath,
				},
			},
		},
	}

	validateGateway(t, output.Resources, expectedHostname, expectedIncludes, nil)
	validateHttpRoute(t, output.Resources, routeName, httpRoutePort, nil)
}

func renderHttpRoute(t *testing.T, port int32) renderers.RendererOutput {
	r := &httproute.Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	properties := datamodel.HTTPRouteProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Port: port,
	}
	resource := makeDependentResource(t, properties)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{}})
	require.NoError(t, err)

	return output
}

func validateGateway(t *testing.T, outputResources []outputresource.OutputResource, expectedHostname string, expectedIncludes []contourv1.Include, expectedTCPProxy *contourv1.TCPProxy) {
	gateway, gatewayOutputResource := kubernetes.FindGateway(outputResources)

	expectedGatewayOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedGatewayOutputResource, gatewayOutputResource)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), gateway.Name)
	require.Equal(t, applicationName, gateway.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName, ResourceType), gateway.Labels)

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

	if expectedTCPProxy != nil && expectedHostname != "" {
		expectedGatewaySpec.VirtualHost.TLS = &contourv1.TLS{
			Passthrough: true,
		}
		expectedGatewaySpec.TCPProxy = expectedTCPProxy
	}

	require.Equal(t, expectedVirtualHost, gateway.Spec.VirtualHost)
	require.Equal(t, expectedTCPProxy, gateway.Spec.TCPProxy)
	require.Equal(t, expectedGatewaySpec, gateway.Spec)
}

func validateHttpRoute(t *testing.T, outputResources []outputresource.OutputResource, expectedRouteName string, expectedPort int32, expectedRewrite *contourv1.PathRewritePolicy) {
	expectedLocalID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, expectedRouteName)
	httpRoute, httpRouteOutputResource := kubernetes.FindHttpRouteByLocalID(outputResources, expectedLocalID)
	expectedHttpRouteOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, expectedLocalID, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedHttpRouteOutputResource, httpRouteOutputResource)

	require.Equal(t, kubernetes.NormalizeResourceName(expectedRouteName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, expectedRouteName, ResourceType), httpRoute.Labels)

	require.Nil(t, httpRoute.Spec.VirtualHost)

	expectedServiceName := kubernetes.NormalizeResourceName(expectedRouteName)

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
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/gateways/test-gateway",
				Name: resourceName,
				Type: "Applications.Core/gateways",
			},
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
	id, err := resources.ParseResource(resourceID)
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
			Name: kubernetes.NormalizeResourceName(routeName),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routePath,
				},
			},
		},
	}

	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: config.Application,
		},
		Hostname: config.Hostname,
		Routes: []datamodel.GatewayRoute{
			defaultRoute,
		},
	}

	return properties, includes
}

func GetEnvironmentOptions(hostname, externalIP, port string, publicEndpointOverride bool) renderers.EnvironmentOptions {
	environmentOptions := renderers.EnvironmentOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: publicEndpointOverride,
			Hostname:               hostname,
			Port:                   port,
			ExternalIP:             externalIP,
		},
		Namespace: applicationName,
	}
	return environmentOptions
}
