// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutev1alpha3

import (
	"context"
	"fmt"
	"testing"

	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	applicationName = "test-application"
	resourceName    = "test-route"
)

func Test_GetDependencyIDs_Empty(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{}
	dependencies, err := r.GetDependencyIDs(context.Background(), resource)
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

	output, err := r.Render(context.Background(), resource, dependencies)
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

	output, err := r.Render(context.Background(), resource, dependencies)
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
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, dependencies)
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
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	service := backend.ServiceName
	require.NotNil(t, service)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), service)
	require.Equal(t, int32(81), backend.Port)
}

func Test_Render_WithHostname(t *testing.T) {
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

	dependencies := map[string]renderers.RendererDependency{
		"foo-bar": renderers.RendererDependency{},
	}

	output, err := renderer.Render(context.Background(), resource, dependencies)
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)

	httpRoute, outputResource := kubernetes.FindHttpRoute(output.Resources)
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	require.Equal(t, "example.com", httpRoute.Spec.Gateways)
	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), service)
	require.Equal(t, int32(81), backend.Port)
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
			},
		},
	}

	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, dependencies)
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)

	httpRoute, outputResource := kubernetes.FindHttpRoute(output.Resources)
	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDHttpRoute, httpRoute, httpRoute.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), httpRoute.Name)
	require.Equal(t, applicationName, httpRoute.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), httpRoute.Labels)

	rule := httpRoute.Spec.Rules[0]

	require.NotNil(t, rule.Matches)
	require.Len(t, rule.Matches, 1)

	path := rule.Matches[0]

	require.Equal(t, "/", path.Path.Value)
	require.Equal(t, gatewayv1alpha1.PathMatchPrefix, path.Path.Type)

	backend := rule.ForwardTo[0]
	require.NotNil(t, backend)

	service := backend.ServiceName
	require.NotNil(t, service)

	require.Equal(t, kubernetes.MakeResourceName(applicationName, resourceName), service)
	require.Equal(t, int32(81), backend.Port)
}
