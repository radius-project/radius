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
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/resources"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
)

func NewAzureCosmosDBMongoHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosDBMongoHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosDBMongoHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosDBMongoHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, CosmosDBAccountIDKey, CosmosDBDatabaseIDKey)
	if err != nil {
		return nil, err
	}

	var account *documentdb.DatabaseAccountGetResults
	if properties[CosmosDBAccountIDKey] == "" {
		// If we don't have an ID already then we will need to create a new one.
		account, err = handler.CreateCosmosDBAccount(ctx, properties, documentdb.MongoDB)
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
		account, err := handler.CreateDatabase(ctx, *account.Name, properties[CosmosDBDatabaseNameKey], options)
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

func (handler *azureCosmosDBMongoHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	accountName := properties[CosmosDBAccountNameKey]
	dbName := properties[CosmosDBDatabaseNameKey]

	// Delete CosmosDB Mongo database
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

func (handler *azureCosmosDBMongoHandler) GetDatabaseByID(ctx context.Context, databaseID string) (*documentdb.MongoDBDatabaseGetResults, error) {
	parsed, err := radresources.Parse(databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CosmosDB Mongo Database resource id: %w", err)
	}

	mongoClient := documentdb.NewMongoDBResourcesClient(parsed.SubscriptionID)
	mongoClient.Authorizer = handler.arm.Auth

	account, err := mongoClient.GetMongoDBDatabase(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get CosmosDB Mongo Database: %w", err)
	}

	return &account, nil
}

func (handler *azureCosmosDBMongoHandler) CreateDatabase(ctx context.Context, accountName string, dbName string, options PutOptions) (*documentdb.MongoDBDatabaseGetResults, error) {
	mrc := documentdb.NewMongoDBResourcesClient(handler.arm.SubscriptionID)
	mrc.Authorizer = handler.arm.Auth

	dbfuture, err := mrc.CreateUpdateMongoDBDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName, documentdb.MongoDBDatabaseCreateUpdateParameters{
		MongoDBDatabaseCreateUpdateProperties: &documentdb.MongoDBDatabaseCreateUpdateProperties{
			Resource: &documentdb.MongoDBDatabaseResource{
				ID: to.StringPtr(dbName),
			},
			Options: &documentdb.CreateUpdateOptions{
				AutoscaleSettings: &documentdb.AutoscaleSettings{
					MaxThroughput: to.Int32Ptr(DefaultAutoscaleMaxThroughput),
				},
			},
		},
		Tags: map[string]*string{
			resources.TagRadiusApplication: &options.Application,
			resources.TagRadiusComponent:   &options.Component,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	err = dbfuture.WaitForCompletionRef(ctx, mrc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	db, err := dbfuture.Result(mrc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT cosmosdb database: %w", err)
	}

	return &db, nil
}

func (handler *azureCosmosDBMongoHandler) DeleteDatabase(ctx context.Context, accountName string, dbName string) error {
	mrc := documentdb.NewMongoDBResourcesClient(handler.arm.SubscriptionID)
	mrc.Authorizer = handler.arm.Auth

	// It's possible that this is a retry and we already deleted the account on a previous attempt.
	// When that happens a delete for the database (a nested resource) can fail with a 404, but it's
	// benign.
	dbfuture, err := mrc.DeleteMongoDBDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName)
	if err != nil && dbfuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	}

	err = dbfuture.WaitForCompletionRef(ctx, mrc.Client)
	if err != nil && !util.IsAutorest404Error(err) {
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	}

	response, err := dbfuture.Result(mrc)
	if err != nil && response.StatusCode != 404 { // See comment on DeleteMongoDBDatabase
		return fmt.Errorf("failed to DELETE cosmosdb database: %w", err)
	}

	return nil
}
