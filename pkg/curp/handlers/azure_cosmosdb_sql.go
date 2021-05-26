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
	"github.com/Azure/radius/pkg/curp/armauth"
)

func NewAzureCosmosDBSQLHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosDBSQLDBHandler{arm: arm}
}

type azureCosmosDBSQLDBHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureCosmosDBSQLDBHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// There is no clear documentation on this mapping of GlobalDocumentDB to SQL. Used this ARM template example as a reference to verify that this is the right option https://docs.microsoft.com/en-us/azure/cosmos-db/how-to-manage-database-account
	account, err := CreateCosmosDBAccount(ctx, handler.arm, properties, documentdb.GlobalDocumentDB)
	if err != nil {
		return nil, err
	}

	properties[CosmosDBAccountNameKey] = *account.Name
	properties[CosmosDBAccountIDKey] = *account.ID

	sqlClient := documentdb.NewSQLResourcesClient(handler.arm.SubscriptionID)
	sqlClient.Authorizer = handler.arm.Auth

	dbName := properties["name"]
	dbfuture, err := sqlClient.CreateUpdateSQLDatabase(ctx, handler.arm.ResourceGroup, *account.Name, dbName, documentdb.SQLDatabaseCreateUpdateParameters{
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

	properties[CosmosDBNameKey] = *db.Name

	return properties, nil
}

func (handler *azureCosmosDBSQLDBHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	accountname := properties[CosmosDBAccountNameKey]
	dbname := properties[CosmosDBNameKey]

	// Delete SQL database in the CosmosDB account
	sqlClient := documentdb.NewSQLResourcesClient(handler.arm.SubscriptionID)
	sqlClient.Authorizer = handler.arm.Auth

	dbfuture, err := sqlClient.DeleteSQLDatabase(ctx, handler.arm.ResourceGroup, accountname, dbname)
	if err != nil && dbfuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to delete CosmosDB SQL database: %w", err)
	} else if dbfuture.Response().StatusCode != 404 {
		err = dbfuture.WaitForCompletionRef(ctx, sqlClient.Client)
		if err != nil {
			return fmt.Errorf("failed to delete CosmosDB SQL database: %w", err)
		}

		response, err := dbfuture.Result(sqlClient)
		if err != nil && response.StatusCode != 404 {
			return fmt.Errorf("failed to delete CosmosDB SQL database: %w", err)
		}
	}

	// Delete CosmosDB account
	err = DeleteCosmosDBAccount(ctx, handler.arm, accountname)
	if err != nil {
		return err
	}

	return nil
}
