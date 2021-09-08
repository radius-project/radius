// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package redisv1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/stretchr/testify/require"
)

func Test_Render_Managed_Azure_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": true,
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, outputresource.LocalIDAzureRedis, resource.LocalID)
	require.Equal(t, resourcekinds.KindAzureRedis, resource.Kind)

	expected := map[string]string{
		handlers.ManagedKey:    "true",
		handlers.RedisBaseName: "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}

func TestInvalidAzureComponentKindFailure(t *testing.T) {
	renderer := AzureRenderer{}

	workload := workloads.InstantiatedWorkload{
		Workload: components.GenericComponent{
			Name: "test-component",
			Kind: "foo",
		},
	}

	_, err := renderer.Render(context.Background(), workload)
	require.Error(t, err)
	require.Equal(t, "the component was expected to have kind 'redislabs.com/Redis@v1alpha1', instead it is 'foo'", err.Error())
}

func Test_Render_AzureRedis_Unmanaged_Failure(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": false,
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, "only managed = true is support for azure redis workload", err.Error())
}
