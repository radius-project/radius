// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containerv1alpha1

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{}

	w := workloads.InstantiatedWorkload{}
	w.Workload = unstructuredFromJSON(t, `
{
	"kind": "Container",
	"apiVersion": "radius.dev/v1alpha1",
	"metadata": {
		"name": "test-container",
		"namespace": "test-app"
	},
	"spec": {
		"container": {
			"image": "test/test-image:latest"
		}
	},
	"provides": [
		{
			"name": "test-service",
			"kind": "http",
			"containerPort": 3000
		}
	]
}
`)

	resources, err := renderer.Render(context.Background(), w)
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
		require.Equal(t, "test-service", port.Name)
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
		require.Equal(t, "test-service", port.Name)
		require.Equal(t, v1.ProtocolTCP, port.Protocol)
		require.Equal(t, int32(3000), port.Port)
	})
}

func unstructuredFromJSON(t *testing.T, s string) unstructured.Unstructured {
	object := map[string]interface{}{}
	err := json.Unmarshal([]byte(s), &object)
	require.NoError(t, err)

	return unstructured.Unstructured{Object: object}
}

func findDeployment(resources []workloads.WorkloadResource) *appsv1.Deployment {
	for _, r := range resources {
		if r.Type != "kubernetes" {
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

func findService(resources []workloads.WorkloadResource) *corev1.Service {
	for _, r := range resources {
		if r.Type != "kubernetes" {
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
