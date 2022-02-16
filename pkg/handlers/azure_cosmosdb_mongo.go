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
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/keys"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func NewAzureCosmosDBMongoHandler(arm armauth.ArmConfig) ResourceHandler {
	handler := &azureCosmosDBMongoHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}

	return handler
}

type azureCosmosDBMongoHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosDBMongoHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying an resource
	err := ValidateResourceIDsForResource(properties, CosmosDBDatabaseIDKey)
	if err != nil {
		return nil, err
	}

	// This is mostly called for the side-effect of verifying that the database exists.
	database, err := handler.GetDatabaseByID(ctx, properties[CosmosDBDatabaseIDKey])
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.NewARMIdentity(*database.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))

	return properties, nil
}

func (handler *azureCosmosDBMongoHandler) Delete(ctx context.Context, options DeleteOptions) error {

	return nil
}

func (handler *azureCosmosDBMongoHandler) GetDatabaseByID(ctx context.Context, databaseID string) (*documentdb.MongoDBDatabaseGetResults, error) {
	parsed, err := azresources.Parse(databaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CosmosDB Mongo Database resource id: %w", err)
	}

	mongoClient := clients.NewMongoDBResourcesClient(parsed.SubscriptionID, handler.arm.Auth)

	database, err := mongoClient.GetMongoDBDatabase(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get CosmosDB Mongo Database: %w", err)
	}

	return &database, nil
}

func (handler *azureCosmosDBMongoHandler) CreateDatabase(ctx context.Context, accountName string, dbName string, options PutOptions) (*documentdb.MongoDBDatabaseGetResults, error) {
	mrc := clients.NewMongoDBResourcesClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
		Tags: keys.MakeTagsForRadiusResource(options.ApplicationName, options.ResourceName),
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
	mrc := clients.NewMongoDBResourcesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	// It's possible that this is a retry and we already deleted the account on a previous attempt.
	// When that happens a delete for the database (a nested resource) can fail with a 404, but it's
	// benign.
	future, err := mrc.DeleteMongoDBDatabase(ctx, handler.arm.ResourceGroup, accountName, dbName)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "mongodb database", err)
	}

	err = future.WaitForCompletionRef(ctx, mrc.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "mongodb database", err)
	}

	return nil
}

func NewAzureCosmosDBMongoHealthHandler(arm armauth.ArmConfig) HealthHandler {
	handler := &azureCosmosDBMongoHealthHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}

	return handler
}

type azureCosmosDBMongoHealthHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosDBMongoHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
