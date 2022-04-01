// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscalev1alpha3

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/renderers/containerv1alpha3"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

var _ renderers.Renderer = (*noop)(nil)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the manualscale trait can modify it
	deployment := appsv1.Deployment{}
	resources := []outputresource.OutputResource{outputresource.NewKubernetesOutputResource(resourcekinds.Deployment, outputresource.LocalIDDeployment, &deployment, deployment.ObjectMeta)}
	return renderers.RendererOutput{Resources: resources}, nil
}

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	resource := renderers.RendererResource{
		ApplicationName: "test-application",
		ResourceName:    "test-resource",
		ResourceType:    containerv1alpha3.ResourceType,
		Definition: map[string]interface{}{
			"traits": []map[string]interface{}{
				{
					"kind":     Kind,
					"replicas": 2,
				},
			},
		},
	}
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Equal(t, int32(2), *deployment.Spec.Replicas)
}

func Test_Render_CanSpecifyZero(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	resource := renderers.RendererResource{
		ApplicationName: "test-application",
		ResourceName:    "test-resource",
		ResourceType:    containerv1alpha3.ResourceType,
		Definition: map[string]interface{}{
			"traits": []map[string]interface{}{
				{
					"kind":     Kind,
					"replicas": 0,
				},
			},
		},
	}
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Equal(t, int32(0), *deployment.Spec.Replicas)
}

func Test_Render_NoTrait(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	resource := renderers.RendererResource{
		ApplicationName: "test-application",
		ResourceName:    "test-resource",
		ResourceType:    containerv1alpha3.ResourceType,
		Definition:      map[string]interface{}{},
	}
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Nil(t, deployment.Spec.Replicas)
}
