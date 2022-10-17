// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func NewAzureCosmosDBMongoHandler(arm *armauth.ArmConfig) ResourceHandler {
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

// Validates resource exists since Radius does not create underlying Azure resources currently.
func (handler *azureCosmosDBMongoHandler) Put(ctx context.Context, resource *outputresource.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("missing required properties for resource")
	}

	parsedID, err := resources.ParseResource(properties[CosmosDBDatabaseIDKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to parse CosmosDB Mongo Database resource id: %w", err)
	}

	mongoClient := clients.NewMongoDBResourcesClient(parsedID.FindScope(resources.SubscriptionsSegment), handler.arm.Auth)
	database, err := mongoClient.GetMongoDBDatabase(ctx, parsedID.FindScope(resources.ResourceGroupsSegment), properties[CosmosDBAccountNameKey], properties[CosmosDBDatabaseNameKey])
	if err != nil {
		if clients.Is404Error(err) {
			return resourcemodel.ResourceIdentity{}, nil, conv.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure CosmosDB Mongo resource %q does not exist", properties[CosmosDBDatabaseIDKey]))
		}
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to get CosmosDB Mongo Database: %w", err)
	}

	outputResourceIdentity = resourcemodel.NewARMIdentity(&resource.ResourceType, *database.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))

	return outputResourceIdentity, properties, nil
}

func (handler *azureCosmosDBMongoHandler) Delete(ctx context.Context, resource *outputresource.OutputResource) error {
	return nil
}
