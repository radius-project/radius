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
	"github.com/Azure/radius/pkg/curp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer WorkloadRenderer implementation for the CosmosDB for SQL workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate WorkloadRenderer implementation for CosmosDB for SQL workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "microsoft.com/SQL" && service.Kind != "azure.com/CosmosDBSQL" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	if len(wrp) != 1 || wrp[0].Type != workloads.ResourceKindAzureCosmosDBSQL {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindAzureCosmosDBSQL)
	}

	properties := wrp[0].Properties
	accountname := properties[handlers.CosmosDBAccountNameKey]
	dbname := properties[handlers.CosmosDBNameKey]

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

	connectionProperties := map[string]interface{}{
		"connectionString": *(*connectionStrings.ConnectionStrings)[0].ConnectionString,
		"database":         dbname,
	}

	return connectionProperties, nil
}

// Render WorkloadRenderer implementation for CosmosDB for SQL workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := CosmosDBSQLComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if !component.Config.Managed {
		return []workloads.WorkloadResource{}, errors.New("only Radius managed ('managed=true') resources are supported right now")
	}

	// generate data we can use to manage a cosmosdb instance
	resource := workloads.WorkloadResource{
		Type: workloads.ResourceKindAzureCosmosDBSQL,
		Resource: map[string]string{
			"name": w.Workload.Name,
		},
	}

	return []workloads.WorkloadResource{resource}, nil
}
