// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package manualscale

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

type noop struct {
}

func (n *noop) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	return map[string]components.BindingState{}, nil
}

func (n *noop) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	// Return a deployment so the manualscale trait can modify it
	deployment := appsv1.Deployment{}

	deploymentResource := outputresource.OutputResource{
		Resource: &deployment,
		Kind:     outputresource.KindKubernetes,
		LocalID:  outputresource.LocalIDDeployment,
	}

	return []outputresource.OutputResource{deploymentResource}, nil
}

func Test_Render_Success(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	w := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Name: "test-component",
			Kind: "radius.dev/Test@v1alpha1",
			Run:  map[string]interface{}{},
			Traits: []components.GenericTrait{
				{
					Kind: Kind,
					AdditionalProperties: map[string]interface{}{
						"replicas": 2,
					},
				},
			},
		},
	}

	resources, err := renderer.Render(context.Background(), w)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	deployment := findDeployment(resources)
	require.NotNil(t, deployment)

	require.Equal(t, int32(2), *deployment.Spec.Replicas)
}

func Test_Render_CanSpecifyZero(t *testing.T) {
	renderer := &Renderer{Inner: &noop{}}

	w := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Name: "test-component",
			Kind: "radius.dev/Test@v1alpha1",
			Run:  map[string]interface{}{},
			Traits: []components.GenericTrait{
				{
					Kind: Kind,
					AdditionalProperties: map[string]interface{}{
						"replicas": 0,
					},
				},
			},
		},
	}

	resources, err := renderer.Render(context.Background(), w)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	deployment := findDeployment(resources)
	require.NotNil(t, deployment)

	require.Equal(t, int32(0), *deployment.Spec.Replicas)
}

func findDeployment(resources []outputresource.OutputResource) *appsv1.Deployment {
	for _, r := range resources {
		if r.Kind != outputresource.KindKubernetes {
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
