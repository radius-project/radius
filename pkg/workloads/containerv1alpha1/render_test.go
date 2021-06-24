// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

func Test_AllocateBindings_NoHTTPBinding(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-container",
		Workload: components.GenericComponent{
			Name: "test-container",
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "test", // will be ignored
				},
			},
		},
	}

	bindings, err := renderer.AllocateBindings(ctx, w, nil)
	require.NoError(t, err)

	require.Len(t, bindings, 0)
}

func Test_AllocateBindings_HTTPBindings(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-container",
		Workload: components.GenericComponent{
			Name: "test-container",
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"port":       2000,
						"targetPort": 3000,
					},
				},
				"test-binding2": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						// Using default value for port
						"targetPort": 5000,
					},
				},
			},
		},
	}

	bindings, err := renderer.AllocateBindings(ctx, w, nil)
	require.NoError(t, err)

	expected := map[string]components.BindingState{
		"test-binding": {
			Component: "test-container",
			Binding:   "test-binding",
			Kind:      "http",
			Properties: map[string]interface{}{
				"host":   "test-container.test-app.svc.cluster.local",
				"port":   "2000",
				"scheme": "http",
				"uri":    "http://test-container.test-app.svc.cluster.local:2000",
			},
		},
		"test-binding2": {
			Component: "test-container",
			Binding:   "test-binding2",
			Kind:      "http",
			Properties: map[string]interface{}{
				"host":   "test-container.test-app.svc.cluster.local",
				"port":   "80",
				"scheme": "http",
				"uri":    "http://test-container.test-app.svc.cluster.local",
			},
		},
	}
	require.Equal(t, expected, bindings)

}

func Test_Render_Success_DefaultPort(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-container",
		Workload: components.GenericComponent{
			Name: "test-container",
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"targetPort": 3000,
					},
				},
			},
		},
	}

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 2)

	deployment := findDeployment(resources)
	require.NotNil(t, deployment)

	service := findService(resources)
	require.NotNil(t, service)

	labels := map[string]string{
		workloads.LabelRadiusApplication: "test-app",
		workloads.LabelRadiusComponent:   "test-container",
		"app.kubernetes.io/name":         "test-container",
		"app.kubernetes.io/part-of":      "test-app",
		"app.kubernetes.io/managed-by":   "radius-rp",
	}

	matchLabels := map[string]string{
		workloads.LabelRadiusApplication: "test-app",
		workloads.LabelRadiusComponent:   "test-container",
	}

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-container", deployment.Name)
		require.Equal(t, "test-app", deployment.Namespace)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, "test-container", container.Name)
		require.Equal(t, "test/test-image:latest", container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)
		require.Len(t, container.Ports, 1)

		port := container.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(3000), port.ContainerPort)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, "test-container", service.Name)
		require.Equal(t, "test-app", service.Namespace)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(80), port.Port)
		require.Equal(t, intstr.FromInt(3000), port.TargetPort)
	})
}

func Test_Render_Success_NonDefaultPort(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-container",
		Workload: components.GenericComponent{
			Name: "test-container",
			Kind: Kind,
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Bindings: map[string]components.GenericBinding{
				"test-binding": {
					Kind: "http",
					AdditionalProperties: map[string]interface{}{
						"port":       2000,
						"targetPort": 3000,
					},
				},
			},
		},
	}

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 2)

	deployment := findDeployment(resources)
	require.NotNil(t, deployment)

	service := findService(resources)
	require.NotNil(t, service)

	labels := map[string]string{
		workloads.LabelRadiusApplication: "test-app",
		workloads.LabelRadiusComponent:   "test-container",
		"app.kubernetes.io/name":         "test-container",
		"app.kubernetes.io/part-of":      "test-app",
		"app.kubernetes.io/managed-by":   "radius-rp",
	}

	matchLabels := map[string]string{
		workloads.LabelRadiusApplication: "test-app",
		workloads.LabelRadiusComponent:   "test-container",
	}

	t.Run("verify deployment", func(t *testing.T) {
		require.Equal(t, "test-container", deployment.Name)
		require.Equal(t, "test-app", deployment.Namespace)
		require.Equal(t, labels, deployment.Labels)
		require.Empty(t, deployment.Annotations)

		spec := deployment.Spec
		require.Equal(t, matchLabels, spec.Selector.MatchLabels)

		template := spec.Template
		require.Equal(t, labels, template.Labels)
		require.Len(t, template.Spec.Containers, 1)

		container := template.Spec.Containers[0]
		require.Equal(t, "test-container", container.Name)
		require.Equal(t, "test/test-image:latest", container.Image)
		require.Equal(t, v1.PullAlways, container.ImagePullPolicy)
		require.Len(t, container.Ports, 1)

		port := container.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(3000), port.ContainerPort)
	})

	t.Run("verify service", func(t *testing.T) {
		require.Equal(t, "test-container", service.Name)
		require.Equal(t, "test-app", service.Namespace)
		require.Equal(t, labels, service.Labels)
		require.Empty(t, service.Annotations)

		spec := service.Spec
		require.Equal(t, matchLabels, spec.Selector)
		require.Len(t, spec.Ports, 1)

		port := spec.Ports[0]
		require.Equal(t, "test-binding", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(2000), port.Port)
		require.Equal(t, intstr.FromInt(3000), port.TargetPort)
	})
}

func findDeployment(resources []workloads.OutputResource) *appsv1.Deployment {
	for _, r := range resources {
		if !r.IsKubernetesResource() {
			continue
		}

		deployment, ok := r.Resource.(*appsv1.Deployment)
		if !ok {
			continue
		}

		return deployment
	}

	return nil
}

func findService(resources []workloads.OutputResource) *corev1.Service {
	for _, r := range resources {
		if !r.IsKubernetesResource() {
			continue
		}

		service, ok := r.Resource.(*corev1.Service)
		if !ok {
			continue
		}

		return service
	}

	return nil
}
