// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbsqlv1alpha1

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/curp/components"
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer WorkloadRenderer implementation for the CosmosDB for SQL workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate WorkloadRenderer implementation for CosmosDB for SQL workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	if len(resources) != 1 || resources[0].Type != workloads.ResourceKindAzureCosmosDBSQL {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindAzureCosmosDBSQL)
	}

	properties := resources[0].Properties
	accountname := properties[handlers.CosmosDBAccountNameKey]
	dbname := properties[handlers.CosmosDBDatabaseNameKey]

	log.Printf("fulfilling service for account: %v, db: %v", accountname, dbname)

	cosmosDBClient := documentdb.NewDatabaseAccountsClient(r.Arm.SubscriptionID)
	cosmosDBClient.Authorizer = r.Arm.Auth

	connectionStrings, err := cosmosDBClient.ListConnectionStrings(ctx, r.Arm.ResourceGroup, accountname)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if connectionStrings.ConnectionStrings == nil || len(*connectionStrings.ConnectionStrings) == 0 {
		return nil, fmt.Errorf("no connection strings found for cosmos db account: %s", accountname)
	}

	bindings := map[string]components.BindingState{
		"cosmos": {
			Component: workload.Name,
			Binding:   "cosmos",
			Kind:      "azure.com/CosmosDBSQL",
			Properties: map[string]interface{}{
				"connectionString": *(*connectionStrings.ConnectionStrings)[0].ConnectionString,
				"database":         dbname,
			},
		},
		"sql": {
			Component: workload.Name,
			Binding:   "sql",
			Kind:      "microsoft.com/SQL",
			Properties: map[string]interface{}{
				"connectionString": *(*connectionStrings.ConnectionStrings)[0].ConnectionString,
				"database":         dbname,
			},
		},
	}

	return bindings, nil
}

// Render WorkloadRenderer implementation for CosmosDB for SQL workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := CosmosDBSQLComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if component.Config.Managed {
		if component.Config.Resource != "" {
			return nil, errors.New("the 'resource' field cannot be specified when 'managed=true'")
		}

		// generate data we can use to manage a cosmosdb instance
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindAzureCosmosDBSQL,
			Resource: map[string]string{
				handlers.ManagedKey:              "true",
				handlers.CosmosDBAccountBaseName: w.Workload.Name,
				handlers.CosmosDBDatabaseNameKey: w.Workload.Name,
			},
		}

		// It's already in the correct format
		return []workloads.WorkloadResource{resource}, nil
	} else {
		if component.Config.Resource == "" {
			return nil, errors.New("the 'resource' field is required when 'managed' is not specified")
		}

		databaseID, err := resources.Parse(component.Config.Resource)
		if err != nil {
			return nil, errors.New("the 'resource' field must be a valid resource id.")
		}

		err = databaseID.ValidateResourceType(SQLResourceType)
		if err != nil {
			return nil, fmt.Errorf("the 'resource' field must refer to a CosmosDB SQL Database")
		}

		// generate data we can use to connect to a servicebus queue
		resource := workloads.WorkloadResource{
			Type: workloads.ResourceKindAzureCosmosDBSQL,
			Resource: map[string]string{
				handlers.ManagedKey: "false",

				// Truncate the database part of the ID to make an ID for the account
				handlers.CosmosDBAccountIDKey:    resources.MakeID(databaseID.SubscriptionID, databaseID.ResourceGroup, databaseID.Types[0]),
				handlers.CosmosDBDatabaseIDKey:   databaseID.ID,
				handlers.CosmosDBAccountNameKey:  databaseID.Types[0].Name,
				handlers.CosmosDBDatabaseNameKey: databaseID.Types[1].Name,
			},
		}

		// It's already in the correct format
		return []workloads.WorkloadResource{resource}, nil
	}
}
