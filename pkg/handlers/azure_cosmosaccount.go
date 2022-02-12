// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func NewAzureCosmosAccountHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureCosmosAccountHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosAccountHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosAccountHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, CosmosDBAccountIDKey)
	if err != nil {
		return nil, err
	}

	// This is mostly called for the side-effect of verifying that the account exists.
	account, err := handler.GetCosmosDBAccountByID(ctx, properties[CosmosDBAccountIDKey])
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.NewARMIdentity(*account.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))

	return properties, nil
}

func (handler *azureCosmosAccountHandler) Delete(ctx context.Context, options DeleteOptions) error {
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
