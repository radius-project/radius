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
	"github.com/Azure/radius/pkg/rad/util"
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

func (handler *azureCosmosDBSQLDBHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// There is no clear documentation on this mapping of GlobalDocumentDB to SQL.
	// Used this ARM template example as a reference to verify that this is the right option:
	//   https://docs.microsoft.com/en-us/azure/cosmos-db/how-to-manage-database-account
	account, err := handler.CreateCosmosDBAccount(ctx, properties, documentdb.GlobalDocumentDB)
	if err != nil {
		return nil, err
	}

	properties[CosmosDBAccountNameKey] = *account.Name
	properties[CosmosDBAccountIDKey] = *account.ID

	dbName := properties[CosmosDBNameKey]
	db, err := handler.CreateDatabase(ctx, *account.Name, dbName)
	if err != nil {
		return nil, err
	}

	properties[CosmosDBNameKey] = *db.Name

	return properties, nil
}

func (handler *azureCosmosDBSQLDBHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	accountName := properties[CosmosDBAccountNameKey]
	dbName := properties[CosmosDBNameKey]

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

func (handler *azureCosmosDBSQLDBHandler) CreateDatabase(ctx context.Context, accountName string, dbName string) (*documentdb.SQLDatabaseGetResults, error) {
	sqlClient := documentdb.NewSQLResourcesClient(handler.arm.SubscriptionID)
	sqlClient.Authorizer = handler.arm.Auth

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
	sqlClient := documentdb.NewSQLResourcesClient(handler.arm.SubscriptionID)
	sqlClient.Authorizer = handler.arm.Auth

	dbfuture, err := sqlClient.DeleteSQLDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName)
	if err != nil && dbfuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to delete CosmosDB SQL database: %w", err)
	}
	err = dbfuture.WaitForCompletionRef(ctx, sqlClient.Client)
	if err != nil && !util.IsAutorest404Error(err) {
		return fmt.Errorf("failed to delete CosmosDB SQL database: %w", err)
	}

	response, err := dbfuture.Result(sqlClient)
	if err != nil && response.StatusCode != 404 {
		return fmt.Errorf("failed to delete CosmosDB SQL database: %w", err)
	}

	return nil
}
