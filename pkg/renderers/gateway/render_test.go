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
	"github.com/stretchr/testify/require"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	subscriptionID  = "default"
	resourceGroup   = "default"
	applicationName = "test-application"
	resourceName    = "test-gateway"
	gatewayClass    = "gateway-class"
	publicIP        = "86.753.099.99"
	privateIP       = "172.24.0.2"
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

	properties := radclient.GatewayProperties{}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", resourceName, applicationName, publicIP)
	require.Equal(t, output.ComputedValues["hostname"].Value, expectedHostname)

	expectedGatewayHostname := gatewayv1alpha1.Hostname(expectedHostname)
	validateGateway(t, output.Resources, &expectedGatewayHostname)
}

func Test_Render_WithPrefix(t *testing.T) {
	r := &Renderer{}

	prefix := "prefix"
	properties := radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{
			Prefix: &prefix,
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedHostname := fmt.Sprintf("%s.%s.%s.nip.io", prefix, applicationName, publicIP)
	require.Equal(t, output.ComputedValues["hostname"].Value, expectedHostname)

	expectedGatewayHostname := gatewayv1alpha1.Hostname(expectedHostname)
	validateGateway(t, output.Resources, &expectedGatewayHostname)
}

func Test_Render_WithFQHostname(t *testing.T) {
	r := &Renderer{}

	hostname := "test-fqdn.contoso.com"
	properties := radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{
			FullyQualifiedHostname: &hostname,
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)
	require.Equal(t, output.ComputedValues["hostname"].Value, hostname)

	expectedGatewayHostname := gatewayv1alpha1.Hostname(hostname)
	validateGateway(t, output.Resources, &expectedGatewayHostname)
}

func Test_Render_WithFQHostname_OverridesPrefix(t *testing.T) {
	r := &Renderer{}

	hostname := "test-fqdn.contoso.com"
	prefix := "test-prefix"
	properties := radclient.GatewayProperties{
		Hostname: &radclient.GatewayPropertiesHostname{
			Prefix:                 &prefix,
			FullyQualifiedHostname: &hostname,
		},
	}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)
	require.Equal(t, output.ComputedValues["hostname"].Value, hostname)

	expectedGatewayHostname := gatewayv1alpha1.Hostname(hostname)
	validateGateway(t, output.Resources, &expectedGatewayHostname)
}

func Test_Render_WithPrivateIP(t *testing.T) {
	r := &Renderer{}

	properties := radclient.GatewayProperties{}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			GatewayClass: gatewayClass,
			PublicIP:     "172.24.0.2",
		},
	}

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)
	require.Equal(t, output.ComputedValues["hostname"].Value, "unknown")

	validateGateway(t, output.Resources, nil)
}

func Test_Render_WithMissingPublicIP(t *testing.T) {
	r := &Renderer{}

	properties := radclient.GatewayProperties{}
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			GatewayClass: gatewayClass,
			PublicIP:     "",
		},
	}

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)
	require.Equal(t, output.ComputedValues["hostname"].Value, "unknown")

	validateGateway(t, output.Resources, nil)
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

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)
	require.Equal(t, output.ComputedValues["hostname"].Value, expectedHostname)

	expectedGatewayHostname := gatewayv1alpha1.Hostname(expectedHostname)
	gateway := validateGateway(t, output.Resources, &expectedGatewayHostname)
	validateHttpRoute(t, output.Resources, routeName, gateway.Name, path)
}

func Test_Render_Multiple_Routes(t *testing.T) {
	r := &Renderer{}

	var routes []*radclient.GatewayRoute
	routeAName := "routename"
	routeADestination := makeRouteResourceID(routeAName)
	routeAPath := "/"
	routeA := radclient.GatewayRoute{
		Destination: &routeADestination,
		Path:        &routeAPath,
	}
	routeBName := "routename"
	routeBDestination := makeRouteResourceID(routeBName)
	routeBPath := "/"
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

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)
	require.Empty(t, output.SecretValues)
	require.Equal(t, output.ComputedValues["hostname"].Value, expectedHostname)

	expectedGatewayHostname := gatewayv1alpha1.Hostname(expectedHostname)
	gateway := validateGateway(t, output.Resources, &expectedGatewayHostname)
	validateHttpRoute(t, output.Resources, routeAName, gateway.Name, routeAPath)
	validateHttpRoute(t, output.Resources, routeBName, gateway.Name, routeBPath)
}

func validateGateway(t *testing.T, outputResources []outputresource.OutputResource, expectedHostname *gatewayv1alpha1.Hostname) *gatewayv1alpha1.Gateway {
	gateway, gatewayOutputResource := kubernetes.FindGateway(outputResources)

	expectedGatewayOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.Gateway, outputresource.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedGatewayOutputResource, gatewayOutputResource)
	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), gateway.Name)
	require.Equal(t, applicationName, gateway.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), gateway.Labels)

	require.Equal(t, gateway.Spec.GatewayClassName, gatewayClass)

	require.Len(t, gateway.Spec.Listeners, 1)
	expectedListener := gatewayv1alpha1.Listener{
		Hostname: expectedHostname,
		Port:     gatewayv1alpha1.PortNumber(80),
		Protocol: gatewayv1alpha1.HTTPProtocolType,
		Routes: gatewayv1alpha1.RouteBindingSelector{
			Kind: "HTTPRoute",
		},
	}
	require.Equal(t, expectedListener, gateway.Spec.Listeners[0])

	return gateway
}

func validateHttpRoute(t *testing.T, outputResources []outputresource.OutputResource, expectedRouteName, expectedGatewayName, expectedMatchPath string) *gatewayv1alpha1.HTTPRoute {
	expectedLocalID := fmt.Sprintf("%s-%s", outputresource.LocalIDHttpRoute, expectedRouteName)
	httpRoute, httpRouteOutputResource := kubernetes.FindHttpRouteByLocalID(outputResources, expectedLocalID)
	expectedHttpRouteOutputResource := outputresource.NewKubernetesOutputResource(resourcekinds.KubernetesHTTPRoute, expectedLocalID, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedHttpRouteOutputResource, httpRouteOutputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, expectedRouteName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, expectedRouteName), httpRoute.Labels)

	require.Len(t, httpRoute.Spec.Gateways.GatewayRefs, 1)
	require.Equal(t, expectedGatewayName, httpRoute.Spec.Gateways.GatewayRefs[0].Name)

	expectedPathMatch := gatewayv1alpha1.PathMatchPrefix
	expectedPort := gatewayv1alpha1.PortNumber(80)
	expectedServiceName := kubernetes.MakeResourceName(applicationName, expectedRouteName)
	expectedRules := []gatewayv1alpha1.HTTPRouteRule{
		{
			Matches: []gatewayv1alpha1.HTTPRouteMatch{
				{
					Path: &gatewayv1alpha1.HTTPPathMatch{
						Type:  &expectedPathMatch,
						Value: &expectedMatchPath,
					},
				},
			},
			ForwardTo: []gatewayv1alpha1.HTTPRouteForwardTo{
				{
					Port:        &expectedPort,
					ServiceName: &expectedServiceName,
				},
			},
		},
	}
	require.Equal(t, httpRoute.Spec.Rules, expectedRules)

	return httpRoute
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

func GetRuntimeOptions() renderers.RuntimeOptions {
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			GatewayClass: gatewayClass,
			PublicIP:     publicIP,
		},
	}
	return additionalProperties
}
