// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

import (
	"context"
	"testing"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
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

	require.Len(t, resources, 2)
	accountResource := resources[0]
	databaseResource := resources[1]

	require.Equal(t, outputresource.LocalIDAzureCosmosMongoAccount, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosAccountMongo, accountResource.Kind)

	require.Equal(t, outputresource.LocalIDAzureCosmosDBMongo, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosDBMongo, databaseResource.Kind)

	expectedAccount := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.CosmosDBAccountBaseName: "test-component",
	}
	require.Equal(t, expectedAccount, accountResource.Resource)

	expectedDatabase := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.CosmosDBAccountBaseName: "test-component",
		handlers.CosmosDBDatabaseNameKey: "test-component",
	}
	require.Equal(t, expectedDatabase, databaseResource.Resource)
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

	require.Len(t, resources, 2)
	accountResource := resources[0]
	databaseResource := resources[1]

	require.Equal(t, outputresource.LocalIDAzureCosmosMongoAccount, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosAccountMongo, accountResource.Kind)

	require.Equal(t, outputresource.LocalIDAzureCosmosDBMongo, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosDBMongo, databaseResource.Kind)

	expectedAccount := map[string]string{
		handlers.ManagedKey:             "false",
		handlers.CosmosDBAccountIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
		handlers.CosmosDBAccountNameKey: "test-account",
	}
	require.Equal(t, expectedAccount, accountResource.Resource)

	expectedDatabase := map[string]string{
		handlers.ManagedKey:              "false",
		handlers.CosmosDBAccountIDKey:    "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
		handlers.CosmosDBAccountNameKey:  "test-account",
		handlers.CosmosDBDatabaseIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		handlers.CosmosDBDatabaseNameKey: "test-database",
	}
	require.Equal(t, expectedDatabase, databaseResource.Resource)
}

func Test_Render_Unmanaged_MissingResource(t *testing.T) {
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
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
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
