// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

func NewAzureCosmosAccountHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureCosmosAccountHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureCosmosAccountHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureCosmosAccountHandler) Put(ctx context.Context, resource *outputresource.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("missing required properties for resource")
	}

	account, err := handler.GetCosmosDBAccountByID(ctx, properties[CosmosDBAccountIDKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	outputResourceIdentity = resourcemodel.NewARMIdentity(&resource.ResourceType, *account.ID, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()))

	return outputResourceIdentity, properties, nil
}

func (handler *azureCosmosAccountHandler) Delete(ctx context.Context, resource *outputresource.OutputResource) error {
	return nil
}
