// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

import (
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/radius/pkg/radrp/armauth"
	"github.com/Azure/radius/pkg/radrp/components"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the CosmosDB for MongoDB workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for CosmosDB for MongoDB workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	if len(resources) != 1 || resources[0].Type != workloads.ResourceKindAzureCosmosDBMongo {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindAzureCosmosDBMongo)
	}

	properties := resources[0].Properties
	accountname := properties[handlers.CosmosDBAccountNameKey]
	dbname := properties[handlers.CosmosDBDatabaseNameKey]

	log.Printf("fulfilling service for account: %v db: %v", accountname, dbname)

	// cosmos uses the following format for mongo: mongodb://{accountname}:{key}@{endpoint}:{port}/{database}?...{params}
	dac := documentdb.NewDatabaseAccountsClient(r.Arm.SubscriptionID)
	dac.Authorizer = r.Arm.Auth

	css, err := dac.ListConnectionStrings(ctx, r.Arm.ResourceGroup, accountname)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if css.ConnectionStrings == nil || len(*css.ConnectionStrings) == 0 {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	// These connection strings won't include the database
	u, err := url.Parse(*(*css.ConnectionStrings)[0].ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string as a URL: %w", err)
	}

	u.Path = "/" + dbname

	bindings := map[string]components.BindingState{
		"cosmos": {
			Component: workload.Name,
			Binding:   "cosmos",
			Kind:      "azure.com/CosmosDBMongo",
			Properties: map[string]interface{}{
				"connectionString": u.String(),
				"database":         dbname,
			},
		},
		"mongo": {
			Component: workload.Name,
			Binding:   "mongo",
			Kind:      "mongodb.com/Mongo",
			Properties: map[string]interface{}{
				"connectionString": u.String(),
				"database":         dbname,
			},
		},
	}

	return bindings, nil
}

// Render WorkloadRenderer implementation for CosmosDB for MongoDB workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.OutputResource, error) {
	component := CosmosDBMongoComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.OutputResource{}, err
	}

	var resource workloads.OutputResource
	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, workloads.ErrResourceSpecifiedForManagedResource
		}

		// generate data we can use to manage a cosmosdb instance
		resource = workloads.OutputResource{
			ResourceKind: workloads.ResourceKindAzureCosmosDBMongo,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.CosmosDBAccountBaseName: w.Workload.Name,
				handlers.CosmosDBDatabaseNameKey: w.Workload.Name,
			},
			Managed: "true",
		}
	} else {
		if component.Config.Resource == "" {
			return nil, workloads.ErrResourceMissingForUnmanagedResource
		}

		databaseID, err := workloads.ValidateResourceID(component.Config.Resource, MongoResourceType, "CosmosDB Mongo Database")
		if err != nil {
			return nil, err
		}

		// generate data we can use to connect to a servicebus queue
		resource = workloads.OutputResource{
			ResourceKind: workloads.ResourceKindAzureCosmosDBMongo,
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				// Truncate the database part of the ID to make an ID for the account
				handlers.CosmosDBAccountIDKey:    resources.MakeID(databaseID.SubscriptionID, databaseID.ResourceGroup, databaseID.Types[0]),
				handlers.CosmosDBDatabaseIDKey:   databaseID.ID,
				handlers.CosmosDBAccountNameKey:  databaseID.Types[0].Name,
				handlers.CosmosDBDatabaseNameKey: databaseID.Types[1].Name,
			},
		}
	}
	return []workloads.OutputResource{resource}, nil
}
