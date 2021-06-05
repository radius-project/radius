// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dapr

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
)

type noop struct {
}

func (n *noop) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	return map[string]components.BindingState{}, nil
}

func (n *noop) Render(ctx context.Context, workload workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	// Return a deployment so the Dapr trait can modify it
	d := appsv1.Deployment{}
	return []workloads.WorkloadResource{workloads.NewKubernetesResource("Deployment", &d)}, nil
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
						"appId":    "testappId",
						"appPort":  5000,
						"config":   "test-config",
						"protocol": "grpc",
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

	expected := map[string]string{
		"dapr.io/enabled":  "true",
		"dapr.io/app-id":   "testappId",
		"dapr.io/app-port": "5000",
		"dapr.io/protocol": "grpc",
		"dapr.io/config":   "test-config",
	}
	require.Equal(t, expected, deployment.Spec.Template.Annotations)
}

func findDeployment(resources []workloads.WorkloadResource) *appsv1.Deployment {
	for _, r := range resources {
		if !r.IsKubernetesResource() {
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
