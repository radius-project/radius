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
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	"github.com/stretchr/testify/require"
)

const (
	applicationName    = "test-application"
	resourceName       = "test-gateway"
	testExternalIP     = "86.753.099.99"
	testHostname       = "a3cce48e78bc14ae6b0be72e4a33a6e3-797173506.us-west-2.elb.amazonaws.com"
	testPort           = "8080"
	envKubeMetadata    = "EnvKubeMetadata"    // EnvKubeMetadata indicates environment has KubernetesMetadata Extension enabled
	envAppKubeMetadata = "EnvAppKubeMetadata" // AppKubeMetadata indicates both environment and application have KubernetesMetadata Extension enabled

	// User Inputs for testing
	envAnnotationKey1 = "env.ann1"
	envAnnotationVal1 = "env.annval1"

	envLabelKey1 = "env.lbl1"
	envLabelVal1 = "env.lblval1"

	appAnnotationKey1 = "app.ann1"
	appAnnotationVal1 = "app.annval1"

	appLabelKey1 = "app.lbl1"
	appLabelVal1 = "env.lblval1"

	overrideKey1 = "test.ann1"
	overrideKey2 = "test.lbl1"
	overrideVal1 = "override.app.annval1"
	overrideVal2 = "override.app.lblval1"

	managedbyKey    = "app.kubernetes.io/managed-by"
	managedbyVal    = "radius-rp"
	nameKey         = "app.kubernetes.io/name"
	nameRteVal      = "test-route"
	nameGtwyVal     = "test-gateway"
	partofKey       = "app.kubernetes.io/part-of"
	partofVal       = "test-application"
	appKey          = "radius.dev/application"
	appVal          = "test-application"
	resourceKey     = "radius.dev/resource"
	resourceRteVal  = "test-route"
	resourceGtwyVal = "test-gateway"
	resourcetypeKey = "radius.dev/resource-type"
	//resourcetypeRteVal  = "applications.core-httproutes"
	resourcetypeGtwyVal = "applications.core-gateways"
)

type setupMaps struct {
	envKubeMetadataExt *datamodel.KubeMetadataExtension
	appKubeMetadataExt *datamodel.KubeMetadataExtension
}

type expectedMaps struct {
	metaAnn map[string]string
	metaLbl map[string]string
}

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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_WithIPAndPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "prefix"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", prefix, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_WithIPAndFQHostname(t *testing.T) {
	r := &Renderer{}

	expectedHostname := "test-fqdn.contoso.com"
	expectedPublicEndpoint := "http://test-fqdn.contoso.com"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		Hostname: &datamodel.GatewayPropertiesHostname{
			FullyQualifiedHostname: expectedHostname,
		},
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_PublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(testHostname, "", testPort, true, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "http://"+testHostname+":"+testPort, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: testHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_PublicEndpointOverride_OverridesAll(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := "this_CouldbeAnyString"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix:                 "test",
			FullyQualifiedHostname: "testagain",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(expectedPublicEndpoint, testExternalIP, "", true, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "http://"+expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedPublicEndpoint,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_PublicEndpointOverride_WithEmptyIP(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := "www.contoso.com"
	expectedFQDN := "www.contoso.com"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(expectedFQDN, "", "", true, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "http://"+expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedFQDN,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_LocalhostPublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	expectedFQDN := "localhost"
	expectedPublicEndpoint := "http://localhost:8080"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(expectedFQDN, "", testPort, true, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedFQDN,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Hostname(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := fmt.Sprintf("http://%s", testHostname)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(testHostname, "", "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: testHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Hostname_WithPort(t *testing.T) {
	r := &Renderer{}

	expectedFQDN := "www.contoso.com"
	expectedPublicEndpoint := "http://www.contoso.com:32434"
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(expectedFQDN, "", "32434", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedFQDN,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Hostname_WithPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "test"
	expectedFQDN := fmt.Sprintf("%s.%s", prefix, testHostname)
	expectedPublicEndpoint := fmt.Sprintf("http://%s", expectedFQDN)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(testHostname, "", "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedFQDN,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Hostname_WithPrefixAndPort(t *testing.T) {
	r := &Renderer{}

	prefix := "test"
	expectedFQDN := fmt.Sprintf("%s.%s", prefix, testHostname)
	expectedPublicEndpoint := fmt.Sprintf("http://%s:%s", expectedFQDN, testPort)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			Prefix: prefix,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions(testHostname, "", testPort, false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedFQDN,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_WithMissingPublicIP(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(t, properties)
	appId, err := resources.ParseResource(resource.Properties.Application)
	require.NoError(t, err)
	appName := appId.Name()
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", "", "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, "unknown", output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: appName,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			SSLPassthrough: true,
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*v1.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*v1.ErrClientRP).Message, "cannot support `path` or `replacePrefix` in routes with sslPassthrough set to true")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			SSLPassthrough: true,
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*v1.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*v1.ErrClientRP).Message, "cannot support multiple routes with sslPassthrough set to true")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_Fails_WithNoRoute(t *testing.T) {
	r := &Renderer{}

	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*v1.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*v1.ErrClientRP).Message, "must have at least one route when declaring a Gateway resource")
	require.Len(t, output.Resources, 0)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)
}

func Test_Render_FQDNOverride(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := fmt.Sprintf("http://%s", testHostname)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{
			FullyQualifiedHostname: testHostname,
		},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, expectedPublicEndpoint, output.ComputedValues["url"].Value)

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: testHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Fails_WithoutFQHostnameOrPrefix(t *testing.T) {
	r := &Renderer{}

	properties, _ := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{},
	})
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
	validateHttpRoute(t, output.Resources, routeName, 80, nil, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
		TLS:    tls,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "https://" + expectedHostname

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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
			TLS: &contourv1.TLS{
				Passthrough: true,
			},
		},
		TCPProxy: expectedTCPProxy,
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
	validateHttpRoute(t, output.Resources, routeName, 80, nil, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
	validateHttpRoute(t, output.Resources, routeAName, 80, nil, "")
	validateHttpRoute(t, output.Resources, routeBName, 80, nil, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")

	expectedPathRewritePolicy := &contourv1.PathRewritePolicy{
		ReplacePrefix: []contourv1.ReplacePrefix{
			{
				Prefix:      path,
				Replacement: rewrite,
			},
		},
	}
	validateHttpRoute(t, output.Resources, routeName, 80, expectedPathRewritePolicy, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")

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
	validateHttpRoute(t, output.Resources, routeAName, 80, nil, "")
	validateHttpRoute(t, output.Resources, routeBName, 80, expectedPathRewritePolicy, "")
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
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

	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
	validateHttpRoute(t, output.Resources, routeName, httpRoutePort, nil, "")
}

func Test_Render_WithEnvironment_KubernetesMetadata(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeName := "test-route"
	destination := makeRouteResourceID(routeName)
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: destination,
		Path:        path,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, true)
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, envKubeMetadata)
	validateHttpRoute(t, output.Resources, routeName, 80, nil, envKubeMetadata)
}

func Test_Render_WithEnvironmentApplication_KubernetesMetadata(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeName := "test-route"
	destination := makeRouteResourceID(routeName)
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: destination,
		Path:        path,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, true)
	applicationOptions := getApplicationOptions(true)
	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "http://" + expectedHostname

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions, Application: applicationOptions})
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

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, envAppKubeMetadata)
	validateHttpRoute(t, output.Resources, routeName, 80, nil, envAppKubeMetadata)
}

func Test_Render_With_TLSTermination(t *testing.T) {
	r := &Renderer{}

	secretName := "myapp-tls-secret"
	secretStoreResourceId := makeSecretStoreResourceID(secretName)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			Hostname:               "myapp.radapp.dev",
			MinimumProtocolVersion: "1.2",
			CertificateFrom:        secretStoreResourceId,
		},
	})
	resource := makeResource(t, properties)

	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	dependencies := map[string]renderers.RendererDependency{
		(makeResourceID(t, secretStoreResourceId).String()): {
			ResourceID: makeResourceID(t, secretStoreResourceId),
			Resource: &datamodel.SecretStore{
				Properties: &datamodel.SecretStoreProperties{
					Type: "certificate",
					Data: map[string]*datamodel.SecretStoreDataValue{
						"tls.crt": {
							Value: to.Ptr("test-crt"),
						},
						"tls.key": {
							Value: to.Ptr("test-crt"),
						},
					},
				},
			},
			OutputResources: map[string]resourcemodel.ResourceIdentity{
				"Secret": {
					ResourceType: &resourcemodel.ResourceType{
						Type:     "Secret",
						Provider: "kubernetes",
					},
					Data: map[string]any{
						"kind":       "Secret",
						"apiVersion": "v1",
						"name":       secretName,
						"namespace":  environmentOptions.Namespace,
					},
				},
			},
		},
	}

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, testExternalIP)
	expectedURL := "https://" + expectedHostname
	require.Equal(t, expectedURL, output.ComputedValues["url"].Value)
	expectedTLS := &contourv1.TLS{
		MinimumProtocolVersion: "1.2",
		SecretName:             environmentOptions.Namespace + "/" + "myapp-tls-secret",
	}

	expectedGatewaySpec := &contourv1.HTTPProxySpec{
		VirtualHost: &contourv1.VirtualHost{
			Fqdn: expectedHostname,
			TLS:  expectedTLS,
		},
		Includes: expectedIncludes,
	}

	validateGateway(t, output.Resources, expectedGatewaySpec, "")
}

func validateGateway(t *testing.T, outputResources []rpv1.OutputResource, expectedGatewaySpec *contourv1.HTTPProxySpec, kmeOption string) {
	gateway, gatewayOutputResource := kubernetes.FindGateway(outputResources)

	expectedGatewayOutputResource := rpv1.NewKubernetesOutputResource(resourcekinds.Gateway, rpv1.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedGatewayOutputResource, gatewayOutputResource)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), gateway.Name)
	require.Equal(t, applicationName, gateway.Namespace)
	if !(kmeOption == envKubeMetadata || kmeOption == envAppKubeMetadata) {
		require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName, ResourceType), gateway.Labels)
	} else {
		require.Equal(t, getExpectedMaps(true, kmeOption).metaAnn, gateway.Annotations)
		require.Equal(t, getExpectedMaps(true, kmeOption).metaLbl, gateway.Labels)
	}

	require.Equal(t, expectedGatewaySpec, &gateway.Spec)
}

func validateHttpRoute(t *testing.T, outputResources []rpv1.OutputResource, expectedRouteName string, expectedPort int32, expectedRewrite *contourv1.PathRewritePolicy, kmeOption string) {
	expectedLocalID := fmt.Sprintf("%s-%s", rpv1.LocalIDHttpRoute, expectedRouteName)
	httpRoute, httpRouteOutputResource := kubernetes.FindHttpRouteByLocalID(outputResources, expectedLocalID)
	expectedHttpRouteOutputResource := rpv1.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, expectedLocalID, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedHttpRouteOutputResource, httpRouteOutputResource)
	require.Equal(t, kubernetes.NormalizeResourceName(expectedRouteName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	if !(kmeOption == envKubeMetadata || kmeOption == envAppKubeMetadata) {
		require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, expectedRouteName, ResourceType), httpRoute.Labels)
	} else {
		require.Equal(t, getExpectedMaps(false, kmeOption).metaAnn, httpRoute.Annotations)
		require.Equal(t, getExpectedMaps(false, kmeOption).metaLbl, httpRoute.Labels)
	}
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

func renderHttpRoute(t *testing.T, port int32) renderers.RendererOutput {
	r := &httproute.Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	properties := datamodel.HTTPRouteProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Port: port,
	}
	resource := makeDependentResource(t, properties)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: renderers.EnvironmentOptions{}})
	require.NoError(t, err)

	return output
}

func makeRouteResourceID(routeName string) string {
	return "/planes/radius/local/resourcegroups/test-resourcegroup/providers/Applications.Core/httpRoutes/" + routeName
}

func makeSecretStoreResourceID(secretStoreName string) string {
	return "/planes/radius/local/resourcegroups/test-resourcegroup/providers/Applications.Core/secretStores/" + secretStoreName
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
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: config.Application,
		},
		Hostname: config.Hostname,
		Routes: []datamodel.GatewayRoute{
			defaultRoute,
		},
		TLS: config.TLS,
	}

	return properties, includes
}

func getEnvironmentOptions(hostname, externalIP, port string, publicEndpointOverride bool, hasKME bool) renderers.EnvironmentOptions {
	environmentOptions := renderers.EnvironmentOptions{
		Gateway: renderers.GatewayOptions{
			PublicEndpointOverride: publicEndpointOverride,
			Hostname:               hostname,
			Port:                   port,
			ExternalIP:             externalIP,
		},
		Namespace: applicationName,
	}
	if hasKME {
		environmentOptions.KubernetesMetadata = &datamodel.KubeMetadataExtension{
			Annotations: getSetUpMaps(true).envKubeMetadataExt.Annotations,
			Labels:      getSetUpMaps(true).envKubeMetadataExt.Labels,
		}
	}

	return environmentOptions
}

func getApplicationOptions(hasKME bool) renderers.ApplicationOptions {
	applicationOptions := renderers.ApplicationOptions{}

	if hasKME {
		applicationOptions.KubernetesMetadata = &datamodel.KubeMetadataExtension{
			Annotations: getSetUpMaps(false).appKubeMetadataExt.Annotations,
			Labels:      getSetUpMaps(false).appKubeMetadataExt.Labels,
		}
	}

	return applicationOptions
}

func getSetUpMaps(envOnly bool) *setupMaps {
	setupMap := setupMaps{}

	envKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			envAnnotationKey1: envAnnotationVal1,
			overrideKey1:      envAnnotationVal1,
		},
		Labels: map[string]string{
			envLabelKey1: envLabelVal1,
			overrideKey2: envLabelVal1,
		},
	}
	appKubeMetadataExt := &datamodel.KubeMetadataExtension{
		Annotations: map[string]string{
			appAnnotationKey1: appAnnotationVal1,
			overrideKey1:      overrideVal1,
		},
		Labels: map[string]string{
			appLabelKey1: appLabelVal1,
			overrideKey2: overrideVal2,
		},
	}

	setupMap.envKubeMetadataExt = envKubeMetadataExt

	if !envOnly {
		setupMap.appKubeMetadataExt = appKubeMetadataExt
	}

	return &setupMap
}

func getExpectedMaps(isGateway bool, kmeOption string) *expectedMaps {
	if !(kmeOption == envKubeMetadata || kmeOption == envAppKubeMetadata) {
		return nil
	}
	metaAnn := map[string]string{
		envAnnotationKey1: envAnnotationVal1,
		overrideKey1:      envAnnotationVal1,
	}
	metaLbl := map[string]string{
		envLabelKey1:    envLabelVal1,
		overrideKey2:    envLabelVal1,
		managedbyKey:    managedbyVal,
		resourcetypeKey: resourcetypeGtwyVal,
		partofKey:       partofVal,
		appKey:          appVal,
	}

	if isGateway {
		metaLbl[nameKey] = nameGtwyVal
		metaLbl[resourceKey] = resourceGtwyVal
	} else {
		metaLbl[nameKey] = nameRteVal
		metaLbl[resourceKey] = resourceRteVal
	}

	if kmeOption == envAppKubeMetadata {
		metaAnn[appAnnotationKey1] = appAnnotationVal1
		metaAnn[overrideKey1] = overrideVal1

		metaLbl[appLabelKey1] = appLabelVal1
		metaLbl[overrideKey2] = overrideVal2
	}

	return &expectedMaps{
		metaAnn: metaAnn,
		metaLbl: metaLbl,
	}
}
