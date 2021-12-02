// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateway

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/stretchr/testify/require"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
)

const (
	applicationName = "test-application"
	resourceName    = "test-route"
)

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
	require.Empty(t, output.ComputedValues)

	gateway, outputResource := kubernetes.FindGateway(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), gateway.Name)
	require.Equal(t, applicationName, gateway.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), gateway.Labels)

	require.Equal(t, gateway.Spec.GatewayClassName, "gateway-class")
}

func Test_Render_WithListener(t *testing.T) {
	r := &Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"listeners": map[string]interface{}{
				"test-listener": map[string]interface{}{
					"port":     80,
					"protocol": "http",
				},
			},
		},
	}

	dependencies := map[string]renderers.RendererDependency{}
	additionalProperties := GetRuntimeOptions()

	output, err := r.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies, Runtime: additionalProperties})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)
	require.Empty(t, output.ComputedValues)

	gateway, outputResource := kubernetes.FindGateway(output.Resources)

	expectedOutputResource := outputresource.NewKubernetesOutputResource(outputresource.LocalIDGateway, gateway, gateway.ObjectMeta)
	require.Equal(t, expectedOutputResource, outputResource)

	require.Equal(t, kubernetes.MakeResourceName(resource.ApplicationName, resource.ResourceName), gateway.Name)
	require.Equal(t, applicationName, gateway.Namespace)
	require.Equal(t, kubernetes.MakeDescriptiveLabels(applicationName, resourceName), gateway.Labels)

	require.Equal(t, gateway.Spec.GatewayClassName, "gateway-class")

	listener := gateway.Spec.Listeners[0]
	require.Equal(t, gatewayv1alpha1.PortNumber(80), listener.Port)
	require.Equal(t, gatewayv1alpha1.ProtocolType("http"), listener.Protocol)
}

func GetRuntimeOptions() renderers.RuntimeOptions {
	additionalProperties := renderers.RuntimeOptions{
		Gateway: renderers.GatewayOptions{
			GatewayClass: "gateway-class",
		},
	}
	return additionalProperties
}
