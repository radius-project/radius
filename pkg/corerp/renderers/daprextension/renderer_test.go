// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprextension

import (
	"context"
	"testing"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, dm conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the Dapr extension can modify it
	deployment := appsv1.Deployment{}

	deploymentResource := outputresource.OutputResource{
		Resource: &deployment,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.Deployment,
			Provider: providers.ProviderKubernetes,
		},
		LocalID: outputresource.LocalIDDeployment,
	}

	output := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{deploymentResource},
	}

	return output, nil
}

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	ctnrProperties := datamodel.ContainerProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
		Extensions: []datamodel.Extension{{
			Kind: datamodel.DaprSidecar,
			DaprSidecar: &datamodel.DaprSidecarExtension{
				AppID:    "testappId",
				AppPort:  5000,
				Config:   "test-config",
				Protocol: "grpc",
			},
		}},
	}

	resource := makeresource(t, ctnrProperties)
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
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

	ctnrProperties := datamodel.ContainerProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
		Extensions: []datamodel.Extension{{
			Kind: datamodel.DaprSidecar,
			DaprSidecar: &datamodel.DaprSidecarExtension{
				AppPort:  5000,
				Config:   "test-config",
				Protocol: "grpc",
				Provides: "test-route-id",
			},
		}},
	}
	resource := makeresource(t, ctnrProperties)

	dependencies := map[string]renderers.RendererDependency{
		"test-route-id": {
			Definition: map[string]interface{}{
				"appId": "routeappId",
			},
		},
	}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
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

	ctnrProperties := datamodel.ContainerProperties{
		Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
		Extensions: []datamodel.Extension{{
			Kind: datamodel.DaprSidecar,
			DaprSidecar: &datamodel.DaprSidecarExtension{
				AppID:    "testappId",
				AppPort:  5000,
				Config:   "test-config",
				Protocol: "grpc",
				Provides: "test-route-id",
			},
		}},
	}
	resource := makeresource(t, ctnrProperties)

	dependencies := map[string]renderers.RendererDependency{
		"test-route-id": {
			Definition: map[string]interface{}{
				"appId": "routeappId",
			},
		},
	}

	_, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.Error(t, err)
	require.Equal(t, "the appId specified on a daprInvokeHttpRoutes must match the appId specified on the extension. Route: \"routeappId\", Extension: \"testappId\"", err.Error())
}

func makeresource(t *testing.T, properties datamodel.ContainerProperties) *datamodel.ContainerResource {
	resource := datamodel.ContainerResource{
		TrackedResource: apiv1.TrackedResource{
			ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container",
			Name: "test-container",
			Type: "Applications.Core/containers",
		},
		Properties: properties,
	}
	return &resource
}
