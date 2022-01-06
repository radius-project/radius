// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcemodel"
)

func NewAzureCosmosDBSQLHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosDBSQLDBHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosDBSQLDBHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosDBSQLDBHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, CosmosDBAccountIDKey, CosmosDBDatabaseIDKey)
	if err != nil {
		return nil, err
	}

	if properties[CosmosDBDatabaseIDKey] == "" {
		var cosmosDBAccountName string
		if properties, ok := options.DependencyProperties[outputresource.LocalIDAzureCosmosAccount]; ok {
			cosmosDBAccountName = properties[CosmosDBAccountNameKey]
		}

		database, err := handler.CreateDatabase(ctx, cosmosDBAccountName, properties[CosmosDBDatabaseNameKey], *options)
		if err != nil {
			return nil, err
		}

		// store db so we can delete later
		properties[CosmosDBDatabaseIDKey] = *database.ID
		properties[CosmosDBDatabaseNameKey] = *database.Name
		options.Resource.Identity = resourcemodel.NewARMIdentity(*database.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))

	} else {
		// This is mostly called for the side-effect of verifying that the database exists.
		database, err := handler.GetDatabaseByID(ctx, properties[CosmosDBDatabaseIDKey])
		if err != nil {
			return nil, err
		}

		options.Resource.Identity = resourcemodel.NewARMIdentity(*database.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))
	}

	return properties, nil
}

func (handler *azureCosmosDBSQLDBHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	accountName := properties[CosmosDBAccountNameKey]
	dbName := properties[CosmosDBDatabaseNameKey]

	// Delete SQL database in the CosmosDB account
	err := handler.DeleteDatabase(ctx, accountName, dbName)
	if err != nil {
		return err
	}

	return nil
}

func (handler *azureCosmosDBSQLDBHandler) GetDatabaseByID(ctx context.Context, databaseID string) (*documentdb.SQLDatabaseGetResults, error) {
	parsed, err := azresources.Parse(databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CosmosDB SQL Database resource id: %w", err)
	}

	sqlClient := clients.NewSQLResourcesClient(parsed.SubscriptionID, handler.arm.Auth)

	account, err := sqlClient.GetSQLDatabase(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get CosmosDB SQL Database: %w", err)
	}

	return &account, nil
}

func (handler *azureCosmosDBSQLDBHandler) CreateDatabase(ctx context.Context, accountName string, dbName string, options PutOptions) (*documentdb.SQLDatabaseGetResults, error) {
	sqlClient := clients.NewSQLResourcesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	dbfuture, err := sqlClient.CreateUpdateSQLDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName, documentdb.SQLDatabaseCreateUpdateParameters{
		SQLDatabaseCreateUpdateProperties: &documentdb.SQLDatabaseCreateUpdateProperties{
			Resource: &documentdb.SQLDatabaseResource{
				ID: to.StringPtr(dbName),
			},
			Options: &documentdb.CreateUpdateOptions{
				AutoscaleSettings: &documentdb.AutoscaleSettings{
					MaxThroughput: to.Int32Ptr(DefaultAutoscaleMaxThroughput),
				},
			},
		},
		Tags: keys.MakeTagsForRadiusResource(options.ApplicationName, options.ResourceName),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb database: %w", err)
	}

	err = dbfuture.WaitForCompletionRef(ctx, sqlClient.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb database: %w", err)
	}

	db, err := dbfuture.Result(sqlClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb database: %w", err)
	}

	return &db, nil
}

func (handler *azureCosmosDBSQLDBHandler) DeleteDatabase(ctx context.Context, accountName string, dbName string) error {
	sqlClient := clients.NewSQLResourcesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sqlClient.DeleteSQLDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "sql database", err)
	}

	err = future.WaitForCompletionRef(ctx, sqlClient.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "sql database", err)
	}

	return nil
}

func NewAzureCosmosDBSQLHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureCosmosDBSQLDBHealthHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosDBSQLDBHealthHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosDBSQLDBHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
