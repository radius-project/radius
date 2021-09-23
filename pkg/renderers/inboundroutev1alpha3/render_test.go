// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package inboundroutev1alpha3

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
)

type noop struct {
}

func (n *noop) GetDependencyIDs(ctx context.Context, workload renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, errors.New("should not be called in this test")
}

func (n *noop) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	return renderers.RendererOutput{Resources: []outputresource.OutputResource{}}, nil
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

	trait := Trait{
		Kind: Kind,
		AdditionalProperties: map[string]interface{}{
			"binding": "web",
		},
	}
	w := makeContainerComponent(trait)

	dependencies := map[string]renderers.RendererDependency{
		"web": {
			ComputedValues: map[string]interface{}{
				"port": 5000,
			},
		},
	}

	output, err := renderer.Render(ctx, w, dependencies)
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	ingress, resource := findIngress(output.Resources)
	require.NotNil(t, ingress)
	require.NotNil(t, resource)

	require.Equal(t, outputresource.LocalIDIngress, resource.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, resource.Kind)
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
	require.Equal(t, int32(5000), service.Port.Number)
}

func Test_Render_WithHostname(t *testing.T) {
	ctx := createContext(t)
	renderer := &Renderer{
		Inner: &noop{},
	}

	trait := Trait{
		Kind: Kind,
		AdditionalProperties: map[string]interface{}{
			"hostname": "example.com",
			"binding":  "web",
		},
	}

	dependencies := map[string]renderers.RendererDependency{
		"web": {
			ComputedValues: map[string]interface{}{
				"port": 5000,
			},
		},
	}

	w := makeContainerComponent(trait)

	output, err := renderer.Render(ctx, w, dependencies)
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	ingress, resource := findIngress(output.Resources)
	require.NotNil(t, ingress)
	require.NotNil(t, resource)

	require.Equal(t, outputresource.LocalIDIngress, resource.LocalID)
	require.Equal(t, resourcekinds.Kubernetes, resource.Kind)
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
	require.Equal(t, int32(5000), service.Port.Number)
}

// The inboundroute trait doesn't look at much of the data here, just the provides section.
func makeContainerComponent(trait Trait) renderers.RendererResource {
	return renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-container",
		ResourceType:    "test",
		Definition: map[string]interface{}{
			"traits": []interface{}{
				Trait{
					Kind:                 trait.Kind,
					AdditionalProperties: trait.AdditionalProperties,
				},
			},
		},
	}
}

func findIngress(resources []outputresource.OutputResource) (*networkingv1.Ingress, *outputresource.OutputResource) {
	for _, r := range resources {
		if r.Kind != resourcekinds.Kubernetes {
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
