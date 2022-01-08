// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha3

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
			"queue":   "cool-queue",
		},
	}

	result, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)

	require.Len(t, result.Resources, 3)

	deployment, _ := kubernetes.FindDeployment(result.Resources)
	require.NotNil(t, deployment)

	service, _ := kubernetes.FindService(result.Resources)
	require.NotNil(t, service)

	secret, _ := kubernetes.FindSecret(result.Resources)
	require.NotNil(t, secret)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-resource")

	matchLabels := kubernetes.MakeSelectorLabels("test-app", "test-resource")

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-app-test-resource", deployment.Name)
		require.Equal(t, "test-app", deployment.Namespace)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, "rabbitmq", container.Name)
		require.Equal(t, "rabbitmq:latest", container.Image)
		require.Len(t, container.Ports, 1)

		port1 := container.Ports[0]
		require.Equal(t, v1.ProtocolTCP, port1.Protocol)
		require.Equal(t, int32(5672), port1.ContainerPort)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, "test-app-test-resource", service.Name)
		require.Equal(t, "test-app", service.Namespace)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "rabbitmq", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(5672), port.Port)
		require.Equal(t, intstr.FromInt(5672), port.TargetPort)
	})

	t.Run("verify secret", func(t *testing.T) {
		require.Equal(t, "test-resource", secret.Name)
		require.Equal(t, "test-app", service.Namespace)
		require.Equal(t, labels, secret.Labels)
		require.Empty(t, secret.Annotations)

		data := secret.Data
		require.Equal(t, "amqp://test-app-test-resource:5672", string(data[SecretKeyRabbitMQConnectionString]))
	})
}

func TestRenderUnmanaged(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"queue": "cool-queue",
			"secrets": map[string]interface{}{
				"connectionString": "cool-connection-string",
			},
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	assert.NoError(t, err)
	assert.Equal(t, renderers.RendererOutput{
		ComputedValues: map[string]renderers.ComputedValueReference{
			"queue": {
				Value: "cool-queue",
			},
		},
		SecretValues: map[string]renderers.SecretValueReference{
			"connectionString": {
				LocalID:       outputresource.LocalIDScrapedSecret,
				ValueSelector: "connectionString",
			},
		},
	}, output)
}

func TestInvalidKubernetesMissingQueueName(t *testing.T) {
	renderer := Renderer{}

	dependencies := map[string]renderers.RendererDependency{}
	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-resource",
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "queue name must be specified", err.Error())
}
