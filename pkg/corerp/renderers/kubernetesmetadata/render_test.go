package kubernetesmetadata

import (
	"context"
	"testing"

	apiv1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/renderers"
)

var _ renderers.Renderer = (*noop)(nil)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the extension can modify it
	deployment := appsv1.Deployment{}

	deploymentResource := outputresource.OutputResource{
		Resource: &deployment,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.Deployment,
			Provider: resourcemodel.ProviderKubernetes,
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
	ann := map[string]string{
		"test.ann1": "ann1.val",
		"test.ann2": "ann1.val",
		"test.ann3": "ann1.val",
	}

	lbl := map[string]string{
		"test.lbl1": "lbl1.val",
		"test.lbl2": "lbl2.val",
		"test.lbl3": "lbl3.val",
	}

	properties := makeProperties(t)
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Equal(t, ann, deployment.Spec.Template.Annotations)
	require.Equal(t, lbl, deployment.Spec.Template.Labels)
}

func Test_Render_NoExtension(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
	}

	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Nil(t, deployment.Spec.Template.Annotations)
	require.Nil(t, deployment.Spec.Template.Labels)
}

func makeResource(t *testing.T, properties datamodel.ContainerProperties) *datamodel.ContainerResource {
	resource := datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: apiv1.TrackedResource{
				ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container",
				Name: "test-container",
				Type: "Applications.Core/containers",
			},
		},
		Properties: properties,
	}
	return &resource
}

func makeProperties(t *testing.T) datamodel.ContainerProperties {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rp.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
		Extensions: []datamodel.Extension{{
			Kind: datamodel.KubernetesMetadata,
			KubernetesMetadata: &datamodel.BaseKubernetesMetadataExtension{
				Annotations: map[string]string{
					"test.ann1": "ann1.val",
					"test.ann2": "ann1.val",
					"test.ann3": "ann1.val",
				},
				Labels: map[string]string{
					"test.lbl1": "lbl1.val",
					"test.lbl2": "lbl2.val",
					"test.lbl3": "lbl3.val",
				},
			},
		}},
	}
	return properties
}
