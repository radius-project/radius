/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package manualscale

import (
	"context"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcekinds"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

var _ renderers.Renderer = (*noop)(nil)

type noop struct {
}

func (r *noop) GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) ([]resources.ID, []resources.ID, error) {
	return nil, nil, nil
}

func (r *noop) Render(ctx context.Context, dm v1.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	// Return a deployment so the manualscale extension can modify it
	deployment := appsv1.Deployment{}
	resources := []rpv1.OutputResource{rpv1.NewKubernetesOutputResource(resourcekinds.Deployment, rpv1.LocalIDDeployment, &deployment, deployment.ObjectMeta)}
	return renderers.RendererOutput{Resources: resources}, nil
}

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}
	var (
		replicas  int32  = 3
		preplicas *int32 = &replicas
	)

	properties := makeProperties(t, preplicas)
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Equal(t, replicas, *deployment.Spec.Replicas)
}

func Test_Render_CanSpecifyZero(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	var (
		replicas  int32  = 0
		preplicas *int32 = &replicas
	)

	properties := makeProperties(t, preplicas)
	resource := makeResource(t, properties)
	dependencies := map[string]renderers.RendererDependency{}

	output, err := renderer.Render(context.Background(), resource, renderers.RenderOptions{Dependencies: dependencies})
	require.NoError(t, err)
	require.Len(t, output.Resources, 1)

	deployment, _ := kubernetes.FindDeployment(output.Resources)
	require.NotNil(t, deployment)

	require.Equal(t, int32(0), *deployment.Spec.Replicas)
}

func Test_Render_NoExtension(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
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

	require.Nil(t, deployment.Spec.Replicas)
}

func makeResource(t *testing.T, properties datamodel.ContainerProperties) *datamodel.ContainerResource {
	resource := datamodel.ContainerResource{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/test-sub-id/resourceGroups/test-group/providers/Applications.Core/containers/test-container",
				Name: "test-container",
				Type: "Applications.Core/containers",
			},
		},
		Properties: properties,
	}
	return &resource
}

func makeProperties(t *testing.T, replicas *int32) datamodel.ContainerProperties {
	properties := datamodel.ContainerProperties{
		BasicResourceProperties: rpv1.BasicResourceProperties{
			Application: "/subscriptions/test-sub-id/resourceGroups/test-rg/providers/Applications.Core/applications/test-app",
		},
		Container: datamodel.Container{
			Image: "someimage:latest",
		},
		Extensions: []datamodel.Extension{{
			Kind: datamodel.ManualScaling,
			ManualScaling: &datamodel.ManualScalingExtension{
				Replicas: replicas,
			},
		}},
	}
	return properties
}
