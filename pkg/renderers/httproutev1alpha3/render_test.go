// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import (
	"context"
	"fmt"
	"testing"

	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	namespace       = "test-namespace"
	applicationName = "test-application"
	resourceName    = "test-route"
)

var resourceID = azresources.MakeID(
	"kubernetes",
	namespace,
	azresources.ResourceType{Type: "Microsoft.CustomProviders/resourceProviders", Name: "radiusv3"},
	azresources.ResourceType{Type: "Application", Name: applicationName},
	azresources.ResourceType{Type: "Container", Name: resourceName})

func Test_GetDependencyIDs_Empty(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{}
	dependencies, _, err := r.GetDependencyIDs(context.Background(), resource)
	require.NoError(t, err)
	require.Empty(t, dependencies)
}

func Test_Render_Defaults(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition:      map[string]interface{}{},
	}
	dependencies := map[string]renderers.RendererDependency{}

	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]renderers.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: 80},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:80", kubernetes.MakeResourceName(applicationName, resourceName))},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	service, outputResource := kubernetes.FindService(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDService, service, service.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), service.Name)
	require.Equal(t, applicationName, service.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), service.Labels)

	require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, resource.ResourceType, resourceName), service.Spec.Selector)
	require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	require.Len(t, service.Spec.Ports, 1)
	port := service.Spec.Ports[0]

	expectedPort := corev1.ServicePort{
		Name:       resourceName,
		Port:       80,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + resource.ResourceType + resource.ResourceName)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedPort, port)
}

func GetRuntimeOptions() renderers.RuntimeOptions {
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			GatewayClass: "gateway-class",
		},
	}
	return additionalProperties
}

func Test_Render_NonDefaults(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"port": 81,
		},
	}
	dependencies := map[string]renderers.RendererDependency{}

	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	expectedValues := map[string]renderers.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: 81},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:81", kubernetes.MakeResourceName(applicationName, resourceName))},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	service, outputResource := kubernetes.FindService(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDService, service, service.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), service.Name)
	require.Equal(t, applicationName, service.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), service.Labels)

	require.Equal(t, kubernetes.MakeRouteSelectorLabels(applicationName, resource.ResourceType, resourceName), service.Spec.Selector)
	require.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type)

	require.Len(t, service.Spec.Ports, 1)
	port := service.Spec.Ports[0]

	expectedPort := corev1.ServicePort{
		Name:       resourceName,
		Port:       81,
		TargetPort: intstr.FromString(kubernetes.GetShortenedTargetPortName(resource.ApplicationName + resource.ResourceType + resource.ResourceName)),
		Protocol:   "TCP",
	}
	require.Equal(t, expectedPort, port)
}

// No hostname or any other settings, should be using a default backend
func Test_Render_GatewayWithWildcardHostname(t *testing.T) {
	renderer := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"port": 81,
			"gateway": map[string]interface{}{
				"hostname": "*",
				"source":   "foo-bar",
			},
		},
	}

	id, err := azresources.Parse(resourceID)
	require.NoError(t, err)

	dependencies := map[string]renderers.RendererDependency{
		"foo-bar": {
			ResourceID:      id,
			Definition:      map[string]interface{}{},
			ComputedValues:  map[string]interface{}{},
			OutputResources: map[string]resourcemodel.ResourceIdentity{},
		},
	}

	additionalProperties := GetRuntimeOptions()

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)
	require.Empty(t, output.SecretValues)

	// Adding a gateway has no effect on computed values.
	expectedValues := map[string]renderers.ComputedValueReference{
		"host":   {Value: kubernetes.MakeResourceName(applicationName, resourceName)},
		"port":   {Value: 81},
		"scheme": {Value: "http"},
		"url":    {Value: fmt.Sprintf("http://%s:81", kubernetes.MakeResourceName(applicationName, resourceName))},
	}
	require.Equal(t, expectedValues, output.ComputedValues)

	httpRoute, outputResource := kubernetes.FindHttpRoute(output.Resources)

	require.Empty(t, httpRoute.Spec.Hostnames)
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	require.Equal(t, httpRoute.Spec.ParentRefs[0].Name, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, id.Name())))

	rule := httpRoute.Spec.Rules[0]

	require.NotNil(t, rule.Matches)
	require.Len(t, rule.Matches, 1)

	path := rule.Matches[0]

	require.Equal(t, "/", *path.Path.Value)
	require.Equal(t, gatewayv1alpha2.PathMatchPathPrefix, *path.Path.Type)

	backend := rule.BackendRefs[0]
	require.NotNil(t, backend)

	serviceName := backend.Name
	require.NotNil(t, serviceName)

	require.Equal(t, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, resourceName)), serviceName)
	require.Equal(t, gatewayv1alpha2.PortNumber(81), *backend.Port)
}

func Test_Render_WithHostname_NoRule_DefaultToSlash(t *testing.T) {
	renderer := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"port": 81,
			"gateway": map[string]interface{}{
				"hostname": "example.com",
				"source":   "foo-bar",
			},
		},
	}

	id, err := azresources.Parse(resourceID)
	require.NoError(t, err)

	dependencies := map[string]renderers.RendererDependency{
		"foo-bar": {
			ResourceID:      id,
			Definition:      map[string]interface{}{},
			ComputedValues:  map[string]interface{}{},
			OutputResources: map[string]resourcemodel.ResourceIdentity{},
		},
	}

	additionalProperties := GetRuntimeOptions()

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)

	httpRoute, outputResource := kubernetes.FindHttpRoute(output.Resources)
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	require.Equal(t, gatewayv1alpha2.Hostname("example.com"), httpRoute.Spec.Hostnames[0])

	require.Equal(t, httpRoute.Spec.ParentRefs[0].Name, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, id.Name())))

	rule := httpRoute.Spec.Rules[0]

	require.NotNil(t, rule.Matches)
	require.Len(t, rule.Matches, 1)

	path := rule.Matches[0]

	require.Equal(t, "/", *path.Path.Value)
	require.Equal(t, gatewayv1alpha2.PathMatchPathPrefix, *path.Path.Type)

	backend := rule.BackendRefs[0]
	require.NotNil(t, backend)

	service := backend.Name
	require.NotNil(t, service)

	require.Equal(t, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, resourceName)), service)
	require.Equal(t, gatewayv1alpha2.PortNumber(81), *backend.Port)
}

func Test_Render_Rule(t *testing.T) {
	renderer := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"port": 81,
			"gateway": map[string]interface{}{
				"hostname": "example.com",
				"source":   "foo-bar",
				"rules": map[string]interface{}{
					"foo": map[string]interface{}{
						"path": map[string]interface{}{
							"value": "/foo",
							"type":  "exact",
						},
					},
				},
			},
		},
	}

	id, err := azresources.Parse(resourceID)
	require.NoError(t, err)

	dependencies := map[string]renderers.RendererDependency{
		"foo-bar": {
			ResourceID:      id,
			Definition:      map[string]interface{}{},
			ComputedValues:  map[string]interface{}{},
			OutputResources: map[string]resourcemodel.ResourceIdentity{},
		},
	}

	additionalProperties := GetRuntimeOptions()

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)

	httpRoute, outputResource := kubernetes.FindHttpRoute(output.Resources)
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	require.Equal(t, httpRoute.Spec.ParentRefs[0].Name, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, id.Name())))

	rule := httpRoute.Spec.Rules[0]

	require.NotNil(t, rule.Matches)
	require.Len(t, rule.Matches, 1)

	path := rule.Matches[0]

	require.Equal(t, "/foo", *path.Path.Value)
	require.Equal(t, gatewayv1alpha2.PathMatchExact, *path.Path.Type)

	backend := rule.BackendRefs[0]
	require.NotNil(t, backend)

	service := backend.Name
	require.NotNil(t, service)

	require.Equal(t, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, resourceName)), service)
	require.Equal(t, gatewayv1alpha2.PortNumber(81), *backend.Port)
}

func Test_Render_Rule_NoSource(t *testing.T) {
	renderer := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"port": 81,
			"gateway": map[string]interface{}{
				"hostname": "example.com",
				"rules": map[string]interface{}{
					"foo": map[string]interface{}{
						"path": map[string]interface{}{
							"value": "/foo",
							"type":  "exact",
						},
					},
				},
			},
		},
	}

	id, err := azresources.Parse(resourceID)
	require.NoError(t, err)

	dependencies := map[string]renderers.RendererDependency{
		"foo-bar": {
			ResourceID:      id,
			Definition:      map[string]interface{}{},
			ComputedValues:  map[string]interface{}{},
			OutputResources: map[string]resourcemodel.ResourceIdentity{},
		},
	}

	additionalProperties := GetRuntimeOptions()

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 3)

	httpRoute, outputResource := kubernetes.FindHttpRoute(output.Resources)
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	require.Equal(t, httpRoute.Spec.ParentRefs[0].Name, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, id.Name())))

	rule := httpRoute.Spec.Rules[0]

	require.NotNil(t, rule.Matches)
	require.Len(t, rule.Matches, 1)

	path := rule.Matches[0]

	require.Equal(t, "/foo", *path.Path.Value)
	require.Equal(t, gatewayv1alpha2.PathMatchExact, *path.Path.Type)

	backend := rule.BackendRefs[0]
	require.NotNil(t, backend)

	service := backend.Name
	require.NotNil(t, service)

	require.Equal(t, gatewayv1alpha2.ObjectName(kubernetes.MakeResourceName(applicationName, resourceName)), service)
	require.Equal(t, gatewayv1alpha2.PortNumber(81), *backend.Port)

	gateway, expectedGatewayOutputResource := kubernetes.FindGateway(output.Resources)
	expectedGateway := outputresource.NewKubernetesOutputResource(outputresource.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedGateway, expectedGatewayOutputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, id.Name()), gateway.Name)
}
