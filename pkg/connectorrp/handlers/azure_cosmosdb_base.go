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
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type azureCosmosDBBaseHandler struct {
	arm *armauth.ArmConfig
}

// CosmosDB metadata is stored in a properties map, the 'key' constants below track keys for different properties in the map
const (
	// CosmosDBAccountKindKey is used to specify the account type for creation. It should be a value
	// of documentdb.DatabaseAccountKind.
	CosmosDBAccountKindKey = "cosmosaccountkind"

	// CosmosDBAccountNameKey properties map key for CosmosDB account created for the workload
	CosmosDBAccountNameKey = "cosmosaccountname"

	// CosmosDBDatabaseNameKey properties map key for database name created under CosmosDB account
	CosmosDBDatabaseNameKey = "databasename"

	// CosmosDBAccountIDKey properties map key for unique resource identifier of ARM resource of the account
	CosmosDBAccountIDKey = "cosmosaccountid"

	// CosmosDBDatabaseIDKey properties map key for unique resource identifier of ARM resource of the database
	CosmosDBDatabaseIDKey = "databaseid"
)

func (handler *azureCosmosDBBaseHandler) GetCosmosDBAccountByID(ctx context.Context, accountID string) (*documentdb.DatabaseAccountGetResults, error) {
	parsed, err := resources.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CosmosDB Account resource id: %w", err)
	}

	cosmosDBClient := clients.NewDatabaseAccountsClient(parsed.FindScope(resources.SubscriptionsSegment), handler.arm.Auth)

	account, err := cosmosDBClient.Get(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.TypeSegments()[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get CosmosDB Account: %w", err)
	}

	return &account, nil
}
