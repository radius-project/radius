// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbsqlv1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/pkg/workloads/cosmosdbmongov1alpha1"
	"github.com/stretchr/testify/require"
)

const TestComponentName = "db-cosmossql"

func TestRenderManagedResourse(t *testing.T) {
	renderer := &Renderer{}

	workload := workloads.InstantiatedWorkload{
		Workload: components.GenericComponent{
			Name: TestComponentName,
			Kind: Kind,
			Config: map[string]interface{}{
				"managed": true,
			},
		},
	}

	resources, err := renderer.Render(context.Background(), workload)
	require.NoError(t, err)
	require.Len(t, resources, 1)

	renderedResource := resources[0]

	require.Equal(t, "", renderedResource.LocalID)
	require.Equal(t, workloads.ResourceKindAzureCosmosDBSQL, renderedResource.Type)

	expectedProperties := map[string]string{
		"name": TestComponentName,
	}
	require.Equal(t, expectedProperties, renderedResource.Resource)
}

func TestRenderUnmanagedResource_NotSupported(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Workload: components.GenericComponent{
			Name: TestComponentName,
			Kind: Kind,
			Config: map[string]interface{}{
				"managed": false,
			},
		},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "only Radius managed ('managed=true') resources are supported right now", err.Error())
}

func TestExplicitManagedFlagRequired(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Workload: components.GenericComponent{
			Name: TestComponentName,
			Kind: Kind,
		},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "only Radius managed ('managed=true') resources are supported right now", err.Error())
}

func TestInvalidComponentKindFailure(t *testing.T) {
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Workload: components.GenericComponent{
			Name: TestComponentName,
			Kind: cosmosdbmongov1alpha1.Kind,
		},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "the component was expected to have kind 'azure.com/CosmosDBSQL@v1alpha1', instead it is 'azure.com/CosmosDBMongo@v1alpha1'", err.Error())
}
