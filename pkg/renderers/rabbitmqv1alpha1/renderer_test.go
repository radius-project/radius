// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rabbitmqv1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
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

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
			},
		},
		Namespace:     "default",
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)
	require.Len(t, resources, 3)

	deployment, _ := kubernetes.FindDeployment(resources)
	require.NotNil(t, deployment)

	service, _ := kubernetes.FindService(resources)
	require.NotNil(t, service)

	secret, _ := kubernetes.FindSecret(resources)
	require.NotNil(t, secret)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-component")

	matchLabels := kubernetes.MakeSelectorLabels("test-app", "test-component")

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-app-test-component", deployment.Name)
		require.Equal(t, "default", deployment.Namespace)
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
		require.Equal(t, "test-app-test-component", service.Name)
		require.Equal(t, "default", service.Namespace)
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
		require.Equal(t, "test-component", secret.Name)
		require.Equal(t, "default", secret.Namespace)
		require.Equal(t, labels, secret.Labels)
		require.Empty(t, secret.Annotations)

		data := secret.Data
		require.Equal(t, "amqp://test-app-test-component:5672", string(data[SecretKeyRabbitMQConnectionString]))
	})
}

func TestInvalidKubernetesComponentKindFailure(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Workload: components.GenericComponent{
			Name: "test-component",
			Kind: "foo",
		},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "the component was expected to have kind 'rabbitmq.com/MessageQueue@v1alpha1', instead it is 'foo'", err.Error())
}
