// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azclients"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
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
	properties := mergeProperties(*options.Resource, options.Existing)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, CosmosDBAccountIDKey, CosmosDBDatabaseIDKey)
	if err != nil {
		return nil, err
	}

	var account *documentdb.DatabaseAccountGetResults
	if properties[CosmosDBAccountIDKey] == "" {
		// If we don't have an ID already then we will need to create a new one.
		//
		// There is no clear documentation on this mapping of GlobalDocumentDB to SQL.
		// Used this ARM template example as a reference to verify that this is the right option:
		//   https://docs.microsoft.com/en-us/azure/cosmos-db/how-to-manage-database-account
		account, err = handler.CreateCosmosDBAccount(ctx, properties, documentdb.DatabaseAccountKindGlobalDocumentDB, *options)
		if err != nil {
			return nil, err
		}

		// store account so we can delete later
		properties[CosmosDBAccountIDKey] = *account.ID
		properties[CosmosDBAccountNameKey] = *account.Name
	} else {
		// This is mostly called for the side-effect of verifying that the account exists.
		account, err = handler.GetCosmosDBAccountByID(ctx, properties[CosmosDBAccountIDKey])
		if err != nil {
			return nil, err
		}
	}

	if properties[CosmosDBDatabaseIDKey] == "" {
		account, err := handler.CreateDatabase(ctx, *account.Name, properties[CosmosDBDatabaseNameKey], *options)
		if err != nil {
			return nil, err
		}

		// store db so we can delete later
		properties[CosmosDBDatabaseIDKey] = *account.ID
	} else {
		// This is mostly called for the side-effect of verifying that the database exists.
		_, err := handler.GetDatabaseByID(ctx, properties[CosmosDBDatabaseIDKey])
		if err != nil {
			return nil, err
		}
	}

	return properties, nil
}

func (handler *azureCosmosDBSQLDBHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
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

	// Delete CosmosDB account
	err = handler.DeleteCosmosDBAccount(ctx, accountName)
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

	sqlClient := azclients.NewSQLResourcesClient(parsed.SubscriptionID, handler.arm.Auth)

	account, err := sqlClient.GetSQLDatabase(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get CosmosDB SQL Database: %w", err)
	}

	return &account, nil
}

func (handler *azureCosmosDBSQLDBHandler) CreateDatabase(ctx context.Context, accountName string, dbName string, options PutOptions) (*documentdb.SQLDatabaseGetResults, error) {
	sqlClient := azclients.NewSQLResourcesClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
		Tags: keys.MakeTagsForRadiusComponent(options.Application, options.Component),
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
	sqlClient := azclients.NewSQLResourcesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sqlClient.DeleteSQLDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName)
	if azclients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "sql database", err)
	}

	err = future.WaitForCompletionRef(ctx, sqlClient.Client)
	if azclients.IsLongRunning404(err, future.FutureAPI) {
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
