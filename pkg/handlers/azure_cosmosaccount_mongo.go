// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/resourcemodel"
)

func NewAzureCosmosAccountMongoHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosAccountMongoHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosAccountMongoHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosAccountMongoHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.Existing, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, CosmosDBAccountIDKey)
	if err != nil {
		return nil, err
	}

	var account *documentdb.DatabaseAccountGetResults
	if properties[CosmosDBAccountIDKey] == "" {
		// If the account resourceID doesn't exist, then this is a radius managed resource
		account, err = handler.CreateCosmosDBAccount(ctx, properties, documentdb.DatabaseAccountKindMongoDB, *options)
		if err != nil {
			return nil, err
		}

		options.Resource.Identity = resourcemodel.NewARMIdentity(*account.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))
		properties[CosmosDBAccountIDKey] = *account.ID
		properties[CosmosDBAccountNameKey] = *account.Name
	} else {
		// This is mostly called for the side-effect of verifying that the account exists.
		account, err = handler.GetCosmosDBAccountByID(ctx, properties[CosmosDBAccountIDKey])
		if err != nil {
			return nil, err
		}

		options.Resource.Identity = resourcemodel.NewARMIdentity(*account.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))
	}

	return properties, nil
}

func (handler *azureCosmosAccountMongoHandler) Delete(ctx context.Context, options DeleteOptions) error {
	var properties map[string]string
	if options.ExistingOutputResource == nil {
		properties = options.Existing.Properties
	} else {
		properties = options.ExistingOutputResource.PersistedProperties
	}

	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	// Delete CosmosDB account
	accountName := properties[CosmosDBAccountNameKey]
	err := handler.DeleteCosmosDBAccount(ctx, accountName)
	if err != nil {
		return err
	}

	return nil
}

func NewAzureCosmosAccountMongoHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureCosmosAccountMongoHealthHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosAccountMongoHealthHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosAccountMongoHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
