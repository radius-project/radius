// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render_Managed_Kubernetes_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-redis",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{
		Resource:     resource,
		Dependencies: map[string]renderers.RendererDependency{},
	})
	require.NoError(t, err)
	require.NoError(t, err)
	require.Len(t, output.Resources, 2)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	service, _ := kubernetes.FindService(output.Resources)
	require.NotNil(t, service)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-redis")

	matchLabels := kubernetes.MakeSelectorLabels("test-app", "test-redis")

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-app-test-redis", deployment.Name)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, "redis", container.Name)
		require.Equal(t, "redis", container.Image)
		require.Len(t, container.Ports, 1)

		port := container.Ports[0]
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(6379), port.ContainerPort)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, "test-app-test-redis", service.Name)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "redis", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(6379), port.Port)
		require.Equal(t, intstr.FromInt(6379), port.TargetPort)
	})

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"host": {
			Value: "test-app-test-redis",
		},
		"port": {
			Value: "6379",
		},
		"username": {
			Value: "",
		},
		"password": {
			Value: "",
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Empty(t, output.SecretValues)
}

func TestRenderUnmanagedRedis(t *testing.T) {
	ctx := createContext(t)
	renderer := KubernetesRenderer{}

	input := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-redis",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"host": "hello.com",
			"port": 1234,
			"secrets": map[string]interface{}{
				"connectionString": "***",
				"password":         "***",
			},
		},
	}
	output, err := renderer.Render(ctx, renderers.RenderOptions{
		Resource:     input,
		Dependencies: map[string]renderers.RendererDependency{},
	})
	require.NoError(t, err)

	expected := renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"host": {
				Value: to.StringPtr("hello.com"),
			},
			"port": {
				Value: to.Int32Ptr(1234),
			},
			"username": {
				Value: "",
			},
		},
		SecretValues: map[string]renderers.SecretValueReference{
			"password": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "password",
			},
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}
	assert.DeepEqual(t, expected, output)
}
