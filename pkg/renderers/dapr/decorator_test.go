// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

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

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the Dapr trait can modify it
	deployment := appsv1.Deployment{}

	deploymentResource := outputresource.OutputResource{
		Resource:     &deployment,
		ResourceKind: resourcekinds.Kubernetes,
		LocalID:      outputresource.LocalIDDeployment,
	}

	output := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{deploymentResource},
	}

	return output, nil
}

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-container",
		ResourceType:    containerv1alpha3.ResourceType,
		Definition: map[string]interface{}{
			"traits": []interface{}{
				map[string]interface{}{
					"kind":     "dapr.io/Sidecar@v1alpha1",
					"appId":    "testappId",
					"appPort":  5000,
					"config":   "test-config",
					"protocol": "grpc",
				},
			},
		},
	}
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	expected := map[string]string{
		"dapr.io/enabled":  "true",
		"dapr.io/app-id":   "testappId",
		"dapr.io/app-port": "5000",
		"dapr.io/protocol": "grpc",
		"dapr.io/config":   "test-config",
	}
	require.Equal(t, expected, deployment.Spec.Template.Annotations)
}

func Test_Render_Success_AppID_FromRoute(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-container",
		ResourceType:    containerv1alpha3.ResourceType,
		Definition: map[string]interface{}{
			"traits": []interface{}{
				map[string]interface{}{
					"kind":     "dapr.io/Sidecar@v1alpha1",
					"appPort":  5000,
					"config":   "test-config",
					"protocol": "grpc",
					"provides": "test-route-id",
				},
			},
		},
	}
	dependencies := map[string]renderers.RendererDependency{
		"test-route-id": {
			Definition: map[string]interface{}{
				"appId": "routeappId",
			},
		},
	}

	output, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)
	require.Empty(t, output.SecretValues)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	expected := map[string]string{
		"dapr.io/enabled":  "true",
		"dapr.io/app-id":   "routeappId",
		"dapr.io/app-port": "5000",
		"dapr.io/protocol": "grpc",
		"dapr.io/config":   "test-config",
	}
	require.Equal(t, expected, deployment.Spec.Template.Annotations)
}

func Test_Render_Fail_AppIDFromRouteConflict(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	resource := renderers.RendererResource{
		ApplicationName: "test-app",
		ResourceName:    "test-container",
		ResourceType:    containerv1alpha3.ResourceType,
		Definition: map[string]interface{}{
			"traits": []interface{}{
				map[string]interface{}{
					"kind":     "dapr.io/Sidecar@v1alpha1",
					"appId":    "testappId",
					"appPort":  5000,
					"config":   "test-config",
					"protocol": "grpc",
					"provides": "test-route-id",
				},
			},
		},
	}
	dependencies := map[string]renderers.RendererDependency{
		"test-route-id": {
			Definition: map[string]interface{}{
				"appId": "routeappId",
			},
		},
	}

	_, err := renderer.Render(context.Background(), renderers.RenderOptions{Resource: resource, Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the appId specified on a \"dapr.io.InvokeHttpRoute\" must match the appId specified on the \"dapr.io/Sidecar@v1alpha1\" trait. Route: \"routeappId\", Trait: \"testappId\"", err.Error())
}
