// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/stretchr/testify/require"
)

const (
	applicationName = "test-app"
	resourceName    = "test-db"
)

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Azure_Render_Managed_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": true,
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 2)
	accountResource := output.Resources[0]
	databaseResource := output.Resources[1]

	require.Equal(t, outputresource.LocalIDAzureCosmosAccount, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosAccount, accountResource.ResourceKind)

	require.Equal(t, outputresource.LocalIDAzureCosmosDBMongo, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosDBMongo, databaseResource.ResourceKind)

	expectedAccount := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.CosmosDBAccountBaseName: "test-db",
		handlers.CosmosDBAccountKindKey:  string(documentdb.DatabaseAccountKindMongoDB),
	}
	require.Equal(t, expectedAccount, accountResource.Resource)

	expectedDatabase := map[string]string{
		handlers.ManagedKey:              "true",
		handlers.CosmosDBAccountBaseName: "test-db",
		handlers.CosmosDBDatabaseNameKey: "test-db",
	}
	require.Equal(t, expectedDatabase, databaseResource.Resource)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]renderers.SecretValueReference{
		ConnectionStringValue: {
			LocalID:       cosmosAccountDependency.LocalID,
			Action:        "listConnectionStrings",
			ValueSelector: "/connectionStrings/0/connectionString",
			Transformer:   resourcekinds.AzureCosmosDBMongo,
		},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Azure_Render_Unmanaged_Success(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)

	require.Len(t, output.Resources, 2)
	accountResource := output.Resources[0]
	databaseResource := output.Resources[1]

	require.Equal(t, outputresource.LocalIDAzureCosmosAccount, accountResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosAccount, accountResource.ResourceKind)

	require.Equal(t, outputresource.LocalIDAzureCosmosDBMongo, databaseResource.LocalID)
	require.Equal(t, resourcekinds.AzureCosmosDBMongo, databaseResource.ResourceKind)

	expectedAccount := map[string]string{
		handlers.ManagedKey:             "false",
		handlers.CosmosDBAccountIDKey:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
		handlers.CosmosDBAccountNameKey: "test-account",
		handlers.CosmosDBAccountKindKey: string(documentdb.DatabaseAccountKindMongoDB),
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

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]renderers.SecretValueReference{
		ConnectionStringValue: {
			LocalID:       cosmosAccountDependency.LocalID,
			Action:        "listConnectionStrings",
			ValueSelector: "/connectionStrings/0/connectionString",
			Transformer:   resourcekinds.AzureCosmosDBMongo,
		},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}
func Test_Azure_Render_Unmanaged_UserSpecifiedSecrets(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"secrets": map[string]interface{}{
				"username":         "admin",
				"password":         "deadbeef",
				"connectionString": "admin/deadbeef@localhost",
			},
		},
	}

	output, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.NoError(t, err)
	require.Len(t, output.Resources, 0)
	expectedComputedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
		"password": {
			Value: to.StringPtr("deadbeef"),
		},
		"username": {
			Value: to.StringPtr("admin"),
		},
		"connectionString": {
			Value: to.StringPtr("admin/deadbeef@localhost"),
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, 0, len(output.SecretValues))
}

func Test_Azure_Render_Unmanaged_MissingResource(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"managed": false,
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, renderers.ErrResourceMissingForUnmanagedResource.Error(), err.Error())
}

func Test_Azure_Render_Unmanaged_InvalidResourceType(t *testing.T) {
	ctx := createContext(t)
	renderer := AzureRenderer{}

	resource := renderers.RendererResource{
		ApplicationName: applicationName,
		ResourceName:    resourceName,
		ResourceType:    ResourceType,
		Definition: map[string]interface{}{
			"resource": "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/databaseAccounts/mongodbDatabases/test-database",
		},
	}

	_, err := renderer.Render(ctx, renderers.RenderOptions{Resource: resource, Dependencies: map[string]renderers.RendererDependency{}})
	require.Error(t, err)
	require.Equal(t, "the 'resource' field must refer to a CosmosDB Mongo Database", err.Error())
}
