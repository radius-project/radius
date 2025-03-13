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
	"strings"
	"testing"

	contourv1 "github.com/projectcontour/contour/apis/projectcontour/v1"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/kubernetes"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
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

	managedbyKey        = "app.kubernetes.io/managed-by"
	managedbyVal        = "radius-rp"
	nameKey             = "app.kubernetes.io/name"
	nameRteVal          = "a"
	nameGtwyVal         = "test-gateway"
	partofKey           = "app.kubernetes.io/part-of"
	partofVal           = "test-application"
	appKey              = "radapp.io/application"
	appVal              = "test-application"
	resourceKey         = "radapp.io/resource"
	resourceRteVal      = "a"
	resourceGtwyVal     = "test-gateway"
	resourcetypeKey     = "radapp.io/resource-type"
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

func Test_GetDependencyIDs_Success(t *testing.T) {
	secretStoreID := makeSecretStoreResourceID("testsecret")
	properties := datamodel.GatewayProperties{
		TLS: &datamodel.GatewayPropertiesTLS{
			CertificateFrom: secretStoreID,
		},
		Routes: []datamodel.GatewayRoute{
			{
				Destination: "http://A",
			},
			{
				Destination: "http://B",
			},
		},
	}
	resource := makeResource(properties)

	ctx := testcontext.New(t)
	renderer := Renderer{}
	radiusResourceIDs, resourceIDs, err := renderer.GetDependencyIDs(ctx, resource)
	require.NoError(t, err)
	require.Len(t, radiusResourceIDs, 1)
	require.Len(t, resourceIDs, 0)

	expectedRadiusResourceIDs := []resources.ID{resources.MustParse(secretStoreID)}
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_PublicEndpointOverride(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Hostname(t *testing.T) {
	r := &Renderer{}

	expectedPublicEndpoint := fmt.Sprintf("http://%s", testHostname)
	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_WithMissingPublicIP(t *testing.T) {
	r := &Renderer{}

	properties, expectedIncludes := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
	})
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Fails_SSLPassthroughWithRoutePath(t *testing.T) {
	var routes []datamodel.GatewayRoute
	destination := "http://A"
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
	resource := makeResource(properties)
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
	path := "/"
	route1 := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        path,
	}
	route2 := datamodel.GatewayRoute{
		Destination: "http://B",
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
	resource := makeResource(properties)
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

func Test_Render_WithTimeoutPolicy(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        path,
		TimeoutPolicy: &datamodel.GatewayRouteTimeoutPolicy{
			Request:        "10s",
			BackendRequest: "5s",
		},
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
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

	expectedTimoutPolicy := &contourv1.TimeoutPolicy{
		Response: "10s",
	}

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, nil, expectedTimoutPolicy, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, "")
}

func Test_Render_WithInvalidTimeoutPolicy(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        path,
		TimeoutPolicy: &datamodel.GatewayRouteTimeoutPolicy{
			Request:        "10s",
			BackendRequest: "15s",
		},
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
	dependencies := map[string]renderers.RendererDependency{}
	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	output, err := r.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies, Environment: environmentOptions})
	require.Error(t, err)
	require.Equal(t, err.(*v1.ErrClientRP).Code, v1.CodeInvalid)
	require.Equal(t, err.(*v1.ErrClientRP).Message, "request timeout must be greater than backend request timeout")
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
	resource := makeResource(properties)
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
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
}

func Test_Render_Fails_WithoutFQHostnameOrPrefix(t *testing.T) {
	r := &Renderer{}

	properties, _ := makeTestGateway(datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Hostname: &datamodel.GatewayPropertiesHostname{},
	})
	resource := makeResource(properties)
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
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        path,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
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

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, nil, nil, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, "")
}

func TestRender_SingleRoute_EnableWebsockets(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	path := "/"
	route := datamodel.GatewayRoute{
		Destination:      "http://A",
		Path:             path,
		EnableWebsockets: true,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
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

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, nil, nil, true)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, "")
}

func Test_Render_SSLPassthrough(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	route := datamodel.GatewayRoute{
		Destination: "http://A",
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
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: "/",
				},
			},
		},
	}

	// Create unique localID for dependency graph
	routeResourceName := kubernetes.NormalizeResourceName("A")

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

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, nil, nil, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, "")
}

func Test_Render_Multiple_Routes(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeAPath := "/routea"
	routeA := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        routeAPath,
	}
	routeBPath := "/routeb"
	routeB := datamodel.GatewayRoute{
		Destination: "http://B",
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
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeAPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName("B"),
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

	expectedHTTPRouteSpecA := createExpectedHTTPRouteSpec("A", 80, nil, nil, false)
	expectedHTTPRouteSpecB := createExpectedHTTPRouteSpec("B", 80, nil, nil, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpecA, "")
	validateContourHTTPRoute(t, output.Resources, "B", expectedHTTPRouteSpecB, "")
}

func Test_Render_Route_WithPrefixRewrite(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	path := "/backend"
	rewrite := "/rewrite"
	route := datamodel.GatewayRoute{
		Destination:   "http://A",
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
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")

	expectedPathRewritePolicy := &contourv1.PathRewritePolicy{
		ReplacePrefix: []contourv1.ReplacePrefix{
			{
				Prefix:      path,
				Replacement: rewrite,
			},
		},
	}

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, expectedPathRewritePolicy, nil, false)

	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, "")
}

func Test_Render_Route_WithMultiplePrefixRewrite(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	routeAPath := "/routea"
	routeA := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        routeAPath,
	}
	routeBPath := "/routeb"
	routeBRewrite := "routebrewrite"
	routeB := datamodel.GatewayRoute{
		Destination:   "http://B",
		Path:          routeBPath,
		ReplacePrefix: routeBRewrite,
	}
	routeCPath := "/routec"
	routeCRewrite := "routecrewrite"
	routeC := datamodel.GatewayRoute{
		Destination:   "http://B",
		Path:          routeCPath,
		ReplacePrefix: routeCRewrite,
	}
	routeDPath := "/routed"
	routeD := datamodel.GatewayRoute{
		Destination: "http://B",
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
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeAPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName("B"),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeBPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName("B"),
			Conditions: []contourv1.MatchCondition{
				{
					Prefix: routeCPath,
				},
			},
		},
		{
			Name: kubernetes.NormalizeResourceName("B"),
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")

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

	expectedHTTPRouteSpecA := createExpectedHTTPRouteSpec("A", 80, nil, nil, false)
	expectedHTTPRouteSpecB := createExpectedHTTPRouteSpec("B", 80, expectedPathRewritePolicy, nil, false)

	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpecA, "")
	validateContourHTTPRoute(t, output.Resources, "B", expectedHTTPRouteSpecB, "")
}

func Test_Render_WithDependencies(t *testing.T) {
	r := &Renderer{}

	secret := makeSecretStoreResource(datamodel.SecretStoreProperties{
		Type: datamodel.SecretTypeCert,
		Data: map[string]*datamodel.SecretStoreDataValue{
			"tls.crt": {},
			"tls.key": {},
		},
	})

	var routes []datamodel.GatewayRoute
	routePath := "/routea"

	route := datamodel.GatewayRoute{
		Destination: "http://A:81",
		Path:        routePath,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		TLS: &datamodel.GatewayPropertiesTLS{
			CertificateFrom: secret.ID,
		},
		Routes: routes,
	}
	resource := makeResource(properties)
	dependencies := map[string]renderers.RendererDependency{
		secret.ID: {
			ResourceID:     resources.MustParse(secret.ID),
			Resource:       secret,
			ComputedValues: map[string]any{},
			OutputResources: map[string]resources.ID{
				"Secret": resources_kubernetes.IDFromParts("local", "", "Secret", "default", "test-secret"),
			},
		},
	}

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
			Name: kubernetes.NormalizeResourceName("a"),
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
			TLS: &contourv1.TLS{
				SecretName: "default/test-secret",
			},
		},
		Includes: expectedIncludes,
	}

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 81, nil, nil, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, "")
}

func Test_Render_WithEnvironment_KubernetesMetadata(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        path,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
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

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, nil, nil, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, envKubeMetadata)
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, envKubeMetadata)
}

func Test_Render_WithEnvironmentApplication_KubernetesMetadata(t *testing.T) {
	r := &Renderer{}

	var routes []datamodel.GatewayRoute
	path := "/"
	route := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        path,
	}
	routes = append(routes, route)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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
			Name: kubernetes.NormalizeResourceName("A"),
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

	expectedHTTPRouteSpec := createExpectedHTTPRouteSpec("A", 80, nil, nil, false)

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, envAppKubeMetadata)
	validateContourHTTPRoute(t, output.Resources, "A", expectedHTTPRouteSpec, envAppKubeMetadata)
}

func Test_RenderDNS_WithEnvironmentApplication_KubernetesMetadata(t *testing.T) {
	r := &Renderer{}

	routePathA := "/routea"
	validURL := "http://test-cntr:3234"
	routeA := datamodel.GatewayRoute{
		Destination: validURL,
		Path:        routePathA,
	}

	var routes []datamodel.GatewayRoute
	routeName := "test-cntr"
	path := "/routea"
	routes = append(routes, routeA)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, envAppKubeMetadata)
}

func Test_RenderDNS_WithEnvironment_KubernetesMetadata(t *testing.T) {
	r := &Renderer{}

	routePathA := "/routea"
	validPortDestination := "http://test-cntr:3234"
	routeA := datamodel.GatewayRoute{
		Destination: validPortDestination,
		Path:        routePathA,
	}

	var routes []datamodel.GatewayRoute
	routeName := "test-cntr"
	path := "/routea"
	routes = append(routes, routeA)
	properties := datamodel.GatewayProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-application",
		},
		Routes: routes,
	}
	resource := makeResource(properties)
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, envKubeMetadata)
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
			MinimumProtocolVersion: "1.2",
			CertificateFrom:        secretStoreResourceId,
		},
	})
	resource := makeResource(properties)

	environmentOptions := getEnvironmentOptions("", testExternalIP, "", false, false)

	dependencies := map[string]renderers.RendererDependency{
		secretStoreResourceId: {
			ResourceID: resources.MustParse(secretStoreResourceId),
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
			OutputResources: map[string]resources.ID{
				"Secret": resources_kubernetes.IDFromParts(
					resources_kubernetes.PlaneNameTODO,
					"",
					"Secret",
					environmentOptions.Namespace,
					secretName),
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

	validateContourHTTPProxy(t, output.Resources, expectedGatewaySpec, "")
}

func Test_ParseURL(t *testing.T) {
	const valid_url = "http://examplehost:80"
	const invalid_url = "http://abc:def"
	const invalid_port_url = "http://examplehost:99999"
	const valid_default_http_url = "http://examplehost"
	const valid_default_https_url = "https://examplehost"

	t.Run("valid URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(valid_url)
		require.Equal(t, scheme, "http")
		require.Equal(t, hostname, "examplehost")
		require.Equal(t, port, int32(80))
		require.Equal(t, err, nil)
	})

	t.Run("invalid URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(invalid_url)
		require.Equal(t, scheme, "")
		require.Equal(t, hostname, "")
		require.Equal(t, port, int32(0))
		require.NotEqual(t, err, nil)
	})

	t.Run("invalid port URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(invalid_port_url)
		require.Equal(t, scheme, "")
		require.Equal(t, hostname, "")
		require.Equal(t, port, int32(0))
		require.Equal(t, err, fmt.Errorf("port 0 is out of range"))
	})

	t.Run("valid default http URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(valid_default_http_url)
		require.Equal(t, err, nil)
		require.Equal(t, scheme, "http")
		require.Equal(t, hostname, "examplehost")
		require.Equal(t, port, int32(80))
		require.Equal(t, err, nil)
	})

	t.Run("valid default https URL test", func(t *testing.T) {
		scheme, hostname, port, err := parseURL(valid_default_https_url)
		require.Equal(t, scheme, "https")
		require.Equal(t, hostname, "examplehost")
		require.Equal(t, port, int32(443))
		require.Equal(t, err, nil)
	})
}

func Test_IsURL(t *testing.T) {
	const valid_url = "http://examplehost:80"
	const invalid_url = "http://abc:def"
	const valid_default_http_url = "http://examplehost"
	const valid_default_https_url = "https://examplehost"
	const path = "/testpath/testfolder/testfile.txt"

	require.True(t, isURL(valid_url))
	require.False(t, isURL(invalid_url))
	require.False(t, isURL(path))
	require.True(t, isURL(valid_default_http_url))
	require.True(t, isURL(valid_default_https_url))
}

func validateContourHTTPProxy(t *testing.T, outputResources []rpv1.OutputResource, expectedHTTPProxySpec *contourv1.HTTPProxySpec, kmeOption string) {
	httpProxy, httpProxyOutputResource := kubernetes.FindContourHTTPProxy(outputResources)

	expectedHTTPProxyOutputResource := rpv1.NewKubernetesOutputResource(rpv1.LocalIDGateway, httpProxy, httpProxy.ObjectMeta)
	for _, r := range outputResources {
		if strings.Contains(r.LocalID, rpv1.LocalIDHttpProxy) {
			expectedHTTPProxyOutputResource.CreateResource.Dependencies = append(expectedHTTPProxyOutputResource.CreateResource.Dependencies, r.LocalID)
		}
	}

	// Sort the dependencies so that tests aren't flaky
	slices.Sort(expectedHTTPProxyOutputResource.CreateResource.Dependencies)
	slices.Sort(httpProxyOutputResource.CreateResource.Dependencies)

	require.Equal(t, expectedHTTPProxyOutputResource, httpProxyOutputResource)
	require.Equal(t, kubernetes.NormalizeResourceName(resourceName), httpProxy.Name)
	require.Equal(t, applicationName, httpProxy.Namespace)
	if !(kmeOption == envKubeMetadata || kmeOption == envAppKubeMetadata) {
		require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName, ResourceType), httpProxy.Labels)
	} else {
		require.Equal(t, getExpectedMaps(true, kmeOption).metaAnn, httpProxy.Annotations)
		require.Equal(t, getExpectedMaps(true, kmeOption).metaLbl, httpProxy.Labels)
	}

	require.Equal(t, expectedHTTPProxySpec, &httpProxy.Spec)
}

// validateContourHTTPRoute validates the HTTP route of a Contour resource.
func validateContourHTTPRoute(t *testing.T, outputResources []rpv1.OutputResource, expectedRouteName string, expectedSpec contourv1.HTTPProxySpec, metadataOption string) {
	expectedLocalID := fmt.Sprintf("%s-%s", rpv1.LocalIDHttpProxy, expectedRouteName)
	httpRoute, httpRouteOutputResource := kubernetes.FindContourHTTPProxyByLocalID(outputResources, expectedLocalID)

	// Validate the output resource
	expectedOutputResource := rpv1.NewKubernetesOutputResource(expectedLocalID, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, httpRouteOutputResource)

	// Validate the HTTP route name and namespace
	require.Equal(t, kubernetes.NormalizeResourceName(expectedRouteName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)

	// Validate the metadata
	validateMetadata(t, httpRoute, metadataOption)

	// Validate the HTTP route spec
	require.Equal(t, expectedSpec, httpRoute.Spec)
}

// validateMetadata validates the metadata of a HTTP route.
func validateMetadata(t *testing.T, httpRoute *contourv1.HTTPProxy, metadataOption string) {
	if metadataOption != envKubeMetadata && metadataOption != envAppKubeMetadata {
		expectedLabels := kubernetes.MakeDescriptiveLabels(applicationName, httpRoute.Name, ResourceType)
		require.Equal(t, expectedLabels, httpRoute.Labels)
	} else {
		expectedMaps := getExpectedMaps(false, metadataOption)
		require.Equal(t, expectedMaps.metaAnn, httpRoute.Annotations)
		require.Equal(t, expectedMaps.metaLbl, httpRoute.Labels)
	}
}

// createExpectedHTTPRouteSpec creates the expected HTTP route spec for validation.
func createExpectedHTTPRouteSpec(routeName string, port int32, rewrite *contourv1.PathRewritePolicy, timeout *contourv1.TimeoutPolicy, enableWebsockets bool) contourv1.HTTPProxySpec {
	serviceName := kubernetes.NormalizeResourceName(routeName)
	return contourv1.HTTPProxySpec{
		Routes: []contourv1.Route{
			{
				Services: []contourv1.Service{
					{
						Name: serviceName,
						Port: int(port),
					},
				},
				PathRewritePolicy: rewrite,
				EnableWebsockets:  enableWebsockets,
				TimeoutPolicy:     timeout,
			},
		},
	}
}

func makeSecretStoreResourceID(secretStoreName string) string {
	return "/planes/radius/local/resourcegroups/test-resourcegroup/providers/Applications.Core/secretStores/" + secretStoreName
}

func makeResource(properties datamodel.GatewayProperties) *datamodel.Gateway {
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

func makeSecretStoreResource(properties datamodel.SecretStoreProperties) *datamodel.SecretStore {
	return &datamodel.SecretStore{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/secretStores/test-secretstore",
				Name: "test-secretstore",
			},
		},
		Properties: &properties,
	}
}

func makeTestGateway(config datamodel.GatewayProperties) (datamodel.GatewayProperties, []contourv1.Include) {
	routePath := "/"
	defaultRoute := datamodel.GatewayRoute{
		Destination: "http://A",
		Path:        routePath,
	}

	includes := []contourv1.Include{
		{
			Name: kubernetes.NormalizeResourceName("A"),
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
