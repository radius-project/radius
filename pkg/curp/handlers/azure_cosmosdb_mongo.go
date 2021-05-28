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
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/util"
)

func NewAzureCosmosDBMongoHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosDBMongoHandler{arm: arm}
}

type azureCosmosDBMongoHandler struct {
	arm armauth.ArmConfig
}

func (cddh *azureCosmosDBMongoHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	account, err := CreateCosmosDBAccount(ctx, cddh.arm, properties, documentdb.MongoDB)
	if err != nil {
		return nil, err
	}

	// store account so we can delete later
	properties[CosmosDBAccountIDKey] = *account.ID
	properties[CosmosDBAccountNameKey] = *account.Name

	mrc := documentdb.NewMongoDBResourcesClient(cddh.arm.SubscriptionID)
	mrc.Authorizer = cddh.arm.Auth

	dbName := properties["name"]
	dbfuture, err := mrc.CreateUpdateMongoDBDatabase(ctx, cddh.arm.ResourceGroup, *account.Name, dbName, documentdb.MongoDBDatabaseCreateUpdateParameters{
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

	// store db so we can delete later
	properties[CosmosDBNameKey] = *db.Name

	return properties, nil
}

func (cddh *azureCosmosDBMongoHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	accountname := properties[CosmosDBAccountNameKey]
	dbname := properties[CosmosDBNameKey]

	mrc := documentdb.NewMongoDBResourcesClient(cddh.arm.SubscriptionID)
	mrc.Authorizer = cddh.arm.Auth

	// It's possible that this is a retry and we already deleted the account on a previous attempt.
	// When that happens a delete for the database (a nested resource) can fail with a 404, but it's
	// benign.
	dbfuture, err := mrc.DeleteMongoDBDatabase(ctx, cddh.arm.ResourceGroup, accountname, dbname)
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

	// Delete CosmosDB account
	err = DeleteCosmosDBAccount(ctx, cddh.arm, accountname)
	if err != nil {
		return err
	}

	return nil
}
