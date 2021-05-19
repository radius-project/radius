// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/radius/pkg/curp/armauth"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the CosmosDB for MongoDB workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// Allocate is the WorkloadRenderer implementation for CosmosDB for MongoDB workload.
func (r Renderer) Allocate(ctx context.Context, w workloads.InstantiatedWorkload, wrp []workloads.WorkloadResourceProperties, service workloads.WorkloadService) (map[string]interface{}, error) {
	if service.Kind != "mongodb.com/Mongo" && service.Kind != "azure.com/CosmosDBMongo" {
		return nil, fmt.Errorf("cannot fulfill service kind: %v", service.Kind)
	}

	if len(wrp) != 1 || wrp[0].Type != workloads.ResourceKindAzureCosmosDocumentDB {
		return nil, fmt.Errorf("cannot fulfill service - expected properties for %s", workloads.ResourceKindAzureCosmosDocumentDB)
	}

	properties := wrp[0].Properties
	accountname := properties["cosmosaccountname"]
	dbname := properties["databasename"]

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
		return nil, fmt.Errorf("failed to parse connection string as a URL")
	}

	u.Path = "/" + dbname

	values := map[string]interface{}{
		"connectionString": u.String(),
		"database":         dbname,
	}

	return values, nil
}

// Render WorkloadRenderer implementation for CosmosDB for MongoDB workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]workloads.WorkloadResource, error) {
	component := CosmosDBMongoComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []workloads.WorkloadResource{}, err
	}

	if !component.Config.Managed {
		return []workloads.WorkloadResource{}, errors.New("only 'managed=true' is supported right now")
	}

	// generate data we can use to manage a cosmosdb instance
	resource := workloads.WorkloadResource{
		Type: workloads.ResourceKindAzureCosmosDocumentDB,
		Resource: map[string]string{
			"name": w.Workload.Name,
		},
	}

	// It's already in the correct format
	return []workloads.WorkloadResource{resource}, nil
}
