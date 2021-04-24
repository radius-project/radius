// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroute

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

type noop struct {
}

func (n *noop) Allocate(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	return nil, errors.New("should not be called in this test")
}

func (n *noop) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	return []workloads.WorkloadResource{}, nil
}

// No hostname or any other settings, should be using a default backend
func Test_Render_Simple(t *testing.T) {
	renderer := &Renderer{
		Inner: &noop{},
	}

	trait := components.GenericTrait{
		Kind: Kind,
		Properties: map[string]interface{}{
			"service": "web",
		},
	}
	provides := components.GenericDependency{
		Name: "web",
		Kind: "http",
	}
	w := makeContainerComponent(trait, provides)

	resources, err := renderer.Render(context.Background(), w)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	ingress := findIngress(resources)
	require.NotNil(t, ingress)

	labels := map[string]string{
		workloads.LabelRadiusApplication: "test-app",
		workloads.LabelRadiusComponent:   "test-container",
		"app.kubernetes.io/name":         "test-container",
		"app.kubernetes.io/part-of":      "test-app",
		"app.kubernetes.io/managed-by":   "radius-rp",
	}

	require.Equal(t, "test-container", ingress.Name)
	require.Equal(t, "test-app", ingress.Namespace)
	require.Equal(t, labels, ingress.Labels)
	require.Empty(t, ingress.Annotations)

	require.Empty(t, ingress.Spec.Rules)

	backend := ingress.Spec.DefaultBackend
	require.NotNil(t, backend)

	service := backend.Service
	require.NotNil(t, service)

	require.Equal(t, "test-container", service.Name)
	require.Equal(t, int32(80), service.Port.Number)
}

func Test_Render_WithHostname(t *testing.T) {
	renderer := &Renderer{
		Inner: &noop{},
	}

	trait := components.GenericTrait{
		Kind: Kind,
		Properties: map[string]interface{}{
			"hostname": "example.com",
			"service":  "web",
		},
	}
	provides := components.GenericDependency{
		Name: "web",
		Kind: "http",
	}
	w := makeContainerComponent(trait, provides)

	resources, err := renderer.Render(context.Background(), w)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	ingress := findIngress(resources)
	require.NotNil(t, ingress)

	labels := map[string]string{
		workloads.LabelRadiusApplication: "test-app",
		workloads.LabelRadiusComponent:   "test-container",
		"app.kubernetes.io/name":         "test-container",
		"app.kubernetes.io/part-of":      "test-app",
		"app.kubernetes.io/managed-by":   "radius-rp",
	}

	require.Equal(t, "test-container", ingress.Name)
	require.Equal(t, "test-app", ingress.Namespace)
	require.Equal(t, labels, ingress.Labels)
	require.Empty(t, ingress.Annotations)

	require.Nil(t, ingress.Spec.DefaultBackend)

	require.Len(t, ingress.Spec.Rules, 1)

	rule := ingress.Spec.Rules[0]
	require.Equal(t, "example.com", rule.Host)

	require.NotNil(t, rule.HTTP)
	require.Len(t, rule.HTTP.Paths, 1)

	path := rule.HTTP.Paths[0]
	require.Equal(t, "", path.Path)
	require.Nil(t, path.PathType)

	service := path.Backend.Service
	require.NotNil(t, service)

	require.Equal(t, "test-container", service.Name)
	require.Equal(t, int32(80), service.Port.Number)
}

// The inboundroute trait doesn't look at much of the data here, just the provides section.
func makeContainerComponent(trait components.GenericTrait, httpProvides components.GenericDependency) workloads.InstantiatedWorkload {
	return workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-container",
		Workload: components.GenericComponent{
			Name: "test-container",
			Kind: "radius.dev/Container@v1alpha1",
			Run: map[string]interface{}{
				"container": map[string]interface{}{
					"image": "test/test-image:latest",
				},
			},
			Provides: []components.GenericDependency{
				httpProvides,
			},
			Traits: []components.GenericTrait{
				trait,
			},
		},
	}
}

func findIngress(resources []workloads.WorkloadResource) *networkingv1.Ingress {
	for _, r := range resources {
		if !r.IsKubernetesResource() {
			continue
		}

		ingress, ok := r.Resource.(*networkingv1.Ingress)
		if !ok {
			continue
		}

		return ingress
	}

	return nil
}
