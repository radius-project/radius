// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, _ := radlogger.NewTestLogger(t)
	ctx := logr.NewContext(context.Background(), logger)
	return ctx
}

func Test_Render_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

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

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindAzureCosmosDBMongo, resource.ResourceKind)

	expected := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.CosmosDBAccountBaseName: "test-component",
		handlers.CosmosDBDatabaseNameKey: "test-component",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	resources, err := renderer.Render(ctx, workload)
	require.NoError(t, err)

	require.Len(t, resources, 1)
	resource := resources[0]

	require.Equal(t, "", resource.LocalID)
	require.Equal(t, workloads.ResourceKindAzureCosmosDBMongo, resource.ResourceKind)

	expected := map[string]string{
		handlers.ManagedKey:              "false",
		handlers.CosmosDBAccountIDKey:    "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
		handlers.CosmosDBAccountNameKey:  "test-account",
		handlers.CosmosDBDatabaseIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		handlers.CosmosDBDatabaseNameKey: "test-database",
	}
	require.Equal(t, expected, resource.Resource)
}

func Test_Render_Unmanaged_MissingResourc(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"managed": false,
				// Resource is required
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, workloads.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := Renderer{}

	workload := workloads.InstantiatedWorkload{
		Application: "test-app",
		Name:        "test-component",
		Workload: components.GenericComponent{
			Kind: Kind,
			Name: "test-component",
			Config: map[string]interface{}{
				"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/databaseAccounts/mongodbDatabases/test-database",
			},
		},
		BindingValues: map[components.BindingKey]components.BindingState{},
	}

	_, err := renderer.Render(ctx, workload)
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a CosmosDB Mongo Database", err.Error())
}
