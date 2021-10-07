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
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/keys"
)

type azureCosmosDBBaseHandler struct {
	arm armauth.ArmConfig
}

// CosmosDB metadata is stored in a properties map, the 'key' constants below track keys for different properties in the map
const (
	// CosmosDBAccountKindKey is used to specify the account type for creation. It should be a value
	// of documentdb.DatabaseAccountKind.
	CosmosDBAccountKindKey = "cosmosaccountkind"

	// CosmosDBAccountBaseName is used as the prefix for generated unique account name
	CosmosDBAccountBaseName = "cosmosaccountbasename"

	// CosmosDBAccountNameKey properties map key for CosmosDB account created for the workload
	CosmosDBAccountNameKey = "cosmosaccountname"

	// CosmosDBDatabaseNameKey properties map key for database name created under CosmosDB account
	CosmosDBDatabaseNameKey = "databasename"

	// CosmosDBAccountIDKey properties map key for unique resource identifier of ARM resource of the account
	CosmosDBAccountIDKey = "cosmosaccountid"

	// CosmosDBDatabaseIDKey properties map key for unique resource identifier of ARM resource of the database
	CosmosDBDatabaseIDKey = "databaseid"

	// DefaultAutoscaleMaxThroughput max throughput the database will scale to
	DefaultAutoscaleMaxThroughput = 4000
)

func (handler *azureCosmosDBBaseHandler) GetCosmosDBAccountByID(ctx context.Context, accountID string) (*documentdb.DatabaseAccountGetResults, error) {
	parsed, err := azresources.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CosmosDB Account resource id: %w", err)
	}

	cosmosDBClient := clients.NewDatabaseAccountsClient(parsed.SubscriptionID, handler.arm.Auth)

	account, err := cosmosDBClient.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get CosmosDB Account: %w", err)
	}

	return &account, nil
}

// CreateCosmosDBAccount creates CosmosDB account. Account name is randomly generated with specified database name as prefix.
func (handler *azureCosmosDBBaseHandler) CreateCosmosDBAccount(ctx context.Context, properties map[string]string, databaseKind documentdb.DatabaseAccountKind, options PutOptions) (*documentdb.DatabaseAccountGetResults, error) {
	cosmosDBClient := clients.NewDatabaseAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)
	accountName, ok := properties[CosmosDBAccountNameKey]
	if !ok {
		var err error
		// Generates account name with the specified database name as prefix appended with -<uuid>.
		// This is needed since CosmosDB account names are required to be unique across Azure.
		accountName, err = generateUniqueAzureResourceName(ctx, properties[CosmosDBAccountBaseName], func(name string) error {
			result, err := cosmosDBClient.CheckNameExists(ctx, name)
			if err != nil {
				return fmt.Errorf("failed to query cosmos account name: %w", err)
			}

			if result.StatusCode != 404 {
				return fmt.Errorf("name not available with status code: %v", result.StatusCode)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	accountFuture, err := cosmosDBClient.CreateOrUpdate(ctx, handler.arm.ResourceGroup, accountName, documentdb.DatabaseAccountCreateUpdateParameters{
		Kind:     databaseKind,
		Location: location,
		Tags:     keys.MakeTagsForRadiusResource(options.ApplicationName, options.ResourceName),
		DatabaseAccountCreateUpdateProperties: &documentdb.DatabaseAccountCreateUpdateProperties{
			DatabaseAccountOfferType: to.StringPtr("Standard"), // Standard is the only supported option
			Locations: &[]documentdb.Location{
				{
					LocationName: location,
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, cosmosDBClient.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	account, err := accountFuture.Result(cosmosDBClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	return &account, nil
}

// DeleteCosmosDBAccount deletes CosmosDB account for the specified account name
func (handler *azureCosmosDBBaseHandler) DeleteCosmosDBAccount(ctx context.Context, accountName string) error {
	cosmosDBClient := clients.NewDatabaseAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := cosmosDBClient.Delete(ctx, handler.arm.ResourceGroup, accountName)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "cosmosdb account", err)
	}

	err = future.WaitForCompletionRef(ctx, cosmosDBClient.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "cosmosdb account", err)
	}

	return nil
}
