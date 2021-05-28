package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/gofrs/uuid"
)

// CosmosDB metadata is stored in a properties map, the 'key' constants below track keys for different properties in the map
const (
	// CosmosDBAccountNameKey properties map key for CosmosDB account created for the workload
	CosmosDBAccountNameKey = "cosmosaccountname"

	// CosmosDBNameKey properties map key for database name created under CosmosDB account
	CosmosDBNameKey = "databasename"

	// CosmosDBAccountIDKey properties map key for unique resource identifier of ARM resource
	CosmosDBAccountIDKey = "cosmosaccountid"

	// DefaultAutoscaleMaxThroughput max throughput the database will scale to
	DefaultAutoscaleMaxThroughput = 4000
)

// CreateCosmosDBAccount creates CosmosDB account. Account name is randomly generated with specified database name as prefix.
func CreateCosmosDBAccount(ctx context.Context, armConfig armauth.ArmConfig, properties map[string]string, databaseKind documentdb.DatabaseAccountKind) (account documentdb.DatabaseAccountGetResults, err error) {
	cosmosDBClient := documentdb.NewDatabaseAccountsClient(armConfig.SubscriptionID)
	cosmosDBClient.Authorizer = armConfig.Auth

	accountName, err := generateCosmosDBAccountName(ctx, properties, cosmosDBClient)
	if err != nil {
		return account, err
	}

	rgc := resources.NewGroupsClient(armConfig.SubscriptionID)
	rgc.Authorizer = armConfig.Auth

	rg, err := rgc.Get(ctx, armConfig.ResourceGroup)
	if err != nil {
		return account, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	accountFuture, err := cosmosDBClient.CreateOrUpdate(ctx, armConfig.ResourceGroup, accountName, documentdb.DatabaseAccountCreateUpdateParameters{
		Kind:     databaseKind,
		Location: rg.Location,
		DatabaseAccountCreateUpdateProperties: &documentdb.DatabaseAccountCreateUpdateProperties{
			DatabaseAccountOfferType: to.StringPtr("Standard"), // Standard is the only supported option
			Locations: &[]documentdb.Location{
				{
					LocationName: rg.Location,
				},
			},
		},
	})
	if err != nil {
		return account, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, cosmosDBClient.Client)
	if err != nil {
		return account, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	account, err = accountFuture.Result(cosmosDBClient)
	if err != nil {
		return account, fmt.Errorf("failed to create/update cosmosdb account: %w", err)
	}

	return account, nil
}

// DeleteCosmosDBAccount deletes CosmosDB account for the specified account name
func DeleteCosmosDBAccount(ctx context.Context, armConfig armauth.ArmConfig, accountName string) error {
	cosmosDBClient := documentdb.NewDatabaseAccountsClient(armConfig.SubscriptionID)
	cosmosDBClient.Authorizer = armConfig.Auth

	accountFuture, err := cosmosDBClient.Delete(ctx, armConfig.ResourceGroup, accountName)
	if err != nil {
		return fmt.Errorf("failed to delete cosmosdb account: %w", err)
	}

	err = accountFuture.WaitForCompletionRef(ctx, cosmosDBClient.Client)
	if err != nil && !util.IsAutorest404Error(err) {
		return fmt.Errorf("failed to delete cosmosdb account: %w", err)
	}

	_, err = accountFuture.Result(cosmosDBClient)
	if err != nil {
		return fmt.Errorf("failed to delete cosmosdb account: %w", err)
	}

	return nil
}

// generateCosmosDBAccountName generates account name with the specified database name as prefix appended with -<uuid>.
// This is needed since CosmosDB account names are required to be unique across Azure.
func generateCosmosDBAccountName(ctx context.Context,
	properties map[string]string, cosmosDBClient documentdb.DatabaseAccountsClient) (string, error) {
	retryAttempts := 10
	name, ok := properties[CosmosDBAccountNameKey]
	if !ok {
		// properties["name"] is the component (database) name passed through the template, this is used as a prefix for the account name
		base := properties["name"] + "-"
		name = ""

		for i := 0; i < retryAttempts; i++ {
			// 3-24 characters - all alphanumeric and '-'
			uid, err := uuid.NewV4()
			if err != nil {
				return "", fmt.Errorf("failed to generate CosmosDB account name: %w", err)
			}
			name = base + strings.ReplaceAll(uid.String(), "-", "")
			name = name[0:24]

			result, err := cosmosDBClient.CheckNameExists(ctx, name)
			if err != nil {
				return "", fmt.Errorf("failed to query cosmos account name: %w", err)
			}

			if result.StatusCode == 404 {
				return name, nil
			}

			log.Printf("cosmosDB account name generation failed after %d attempts", i)
		}

		return "", fmt.Errorf("cosmosDB account name generation failed to create a unique name after %d attempts", retryAttempts)
	}

	return name, nil
}
