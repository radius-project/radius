// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroute

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

type noop struct {
}

func (n *noop) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	return nil, errors.New("should not be called in this test")
}

func (n *noop) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	return []outputresource.OutputResource{}, nil
}

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

// No hostname or any other settings, should be using a default backend
func Test_Render_Simple(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{
		Inner: &noop{},
	}

	trait := components.GenericTrait{
		Kind: Kind,
		AdditionalProperties: map[string]interface{}{
			"binding": "web",
		},
	}
	bindings := map[string]components.GenericBinding{
		"web": {
			Kind: "http",
		},
	}
	w := makeContainerComponent(trait, bindings)

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	ingress, resource := findIngress(resources)
	require.NotNil(t, ingress)
	require.NotNil(t, resource)

	require.Equal(t, outputresource.LocalIDIngress, resource.LocalID)
	require.Equal(t, resourcekinds.KindKubernetes, resource.Kind)
	require.Equal(t, outputresource.TypeKubernetes, resource.Type)
	require.True(t, resource.Managed)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-container")

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
	ctx := createContext(t)
	renderer := &Renderer{
		Inner: &noop{},
	}

	trait := components.GenericTrait{
		Kind: Kind,
		AdditionalProperties: map[string]interface{}{
			"hostname": "example.com",
			"binding":  "web",
		},
	}
	bindings := map[string]components.GenericBinding{
		"web": {
			Kind: "http",
		},
	}
	w := makeContainerComponent(trait, bindings)

	resources, err := renderer.Render(ctx, w)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	ingress, resource := findIngress(resources)
	require.NotNil(t, ingress)
	require.NotNil(t, resource)

	require.Equal(t, outputresource.LocalIDIngress, resource.LocalID)
	require.Equal(t, resourcekinds.KindKubernetes, resource.Kind)
	require.Equal(t, outputresource.TypeKubernetes, resource.Type)
	require.True(t, resource.Managed)

	labels := kubernetes.MakeDescriptiveLabels("test-app", "test-container")

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
func makeContainerComponent(trait components.GenericTrait, bindings map[string]components.GenericBinding) workloads.InstantiatedWorkload {
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
			Bindings: bindings,
			Traits: []components.GenericTrait{
				trait,
			},
		},
	}
}

func findIngress(resources []outputresource.OutputResource) (*networkingv1.Ingress, *outputresource.OutputResource) {
	for _, r := range resources {
		if r.Kind != resourcekinds.KindKubernetes {
			continue
		}

		ingress, ok := r.Resource.(*networkingv1.Ingress)
		if !ok {
			continue
		}

		return ingress, &r
	}

	return nil, nil
}
