// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbmongov1alpha1

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/model/components"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/workloads"
)

// Renderer is the WorkloadRenderer implementation for the CosmosDB for MongoDB workload.
type Renderer struct {
	Arm armauth.ArmConfig
}

// AllocateBindings is the WorkloadRenderer implementation for CosmosDB for MongoDB workload.
func (r Renderer) AllocateBindings(ctx context.Context, workload workloads.InstantiatedWorkload, resources []workloads.WorkloadResourceProperties) (map[string]components.BindingState, error) {
	logger := radlogger.GetLogger(ctx)
	if len(workload.Workload.Bindings) > 0 {
		return nil, fmt.Errorf("component of kind %s does not support user-defined bindings", Kind)
	}

	databaseResource, err := workloads.FindByLocalID(resources, outputresource.LocalIDAzureCosmosDBMongo)
	if err != nil {
		return nil, err
	}

	accountName := databaseResource.Properties[handlers.CosmosDBAccountNameKey]
	databaseName := databaseResource.Properties[handlers.CosmosDBDatabaseNameKey]
	logger.Info(fmt.Sprintf("fulfilling binding for account: %v db: %v", accountName, databaseName))

	connectionString, err := GetConnectionString(ctx, r.Arm, accountName, databaseName)
	if err != nil {
		return nil, err
	}

	bindings := map[string]components.BindingState{
		BindingCosmos: {
			Component: workload.Name,
			Binding:   BindingCosmos,
			Kind:      "azure.com/CosmosDBMongo",
			Properties: map[string]interface{}{
				"connectionString": connectionString,
				"database":         databaseName,
			},
		},
		BindingMongo: {
			Component: workload.Name,
			Binding:   BindingMongo,
			Kind:      "mongodb.com/Mongo",
			Properties: map[string]interface{}{
				"connectionString": connectionString,
				"database":         databaseName,
			},
		},
	}

	return bindings, nil
}

// Render WorkloadRenderer implementation for CosmosDB for MongoDB workload.
func (r Renderer) Render(ctx context.Context, w workloads.InstantiatedWorkload) ([]outputresource.OutputResource, error) {
	component := CosmosDBMongoComponent{}
	err := w.Workload.AsRequired(Kind, &component)
	if err != nil {
		return []outputresource.OutputResource{}, err
	}

	if component.Config.Managed {
		return RenderManaged(component)
	} else {
		return RenderUnmanaged(component)
	}
}

func GetConnectionString(ctx context.Context, arm armauth.ArmConfig, accountName string, databaseName string) (string, error) {
	// cosmos uses the following format for mongo: mongodb://{accountname}:{key}@{endpoint}:{port}/{database}?...{params}
	dac := azclients.NewDatabaseAccountsClient(arm.SubscriptionID, arm.Auth)
	css, err := dac.ListConnectionStrings(ctx, arm.ResourceGroup, accountName)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if css.ConnectionStrings == nil || len(*css.ConnectionStrings) == 0 {
		return "", fmt.Errorf("failed to retrieve connection strings")
	}

	// These connection strings won't include the database
	u, err := url.Parse(*(*css.ConnectionStrings)[0].ConnectionString)
	if err != nil {
		return "", fmt.Errorf("failed to parse connection string as a URL: %w", err)
	}

	u.Path = "/" + databaseName
	return u.String(), nil
}

func RenderManaged(component CosmosDBMongoComponent) ([]outputresource.OutputResource, error) {
	if component.Config.Resource != "" {
		return nil, workloads.ErrResourceSpecifiedForManagedResource
	}

	// generate data we can use to manage a cosmosdb instance
	resource := outputresource.OutputResource{
		Kind:    outputresource.KindAzureCosmosDBMongo,
		Type:    outputresource.TypeARM,
		LocalID: outputresource.LocalIDAzureCosmosDBMongo,
		Resource: map[string]string{
			handlers.ManagedKey:              "true",
			handlers.CosmosDBAccountBaseName: component.Name,
			handlers.CosmosDBDatabaseNameKey: component.Name,
		},
		Managed: true,
	}
	return []outputresource.OutputResource{resource}, nil
}

func RenderUnmanaged(component CosmosDBMongoComponent) ([]outputresource.OutputResource, error) {
	if component.Config.Resource == "" {
		return nil, workloads.ErrResourceMissingForUnmanagedResource
	}

	databaseID, err := workloads.ValidateResourceID(component.Config.Resource, MongoResourceType, "CosmosDB Mongo Database")
	if err != nil {
		return nil, err
	}

	resource := outputresource.OutputResource{
		Kind:    outputresource.KindAzureCosmosDBMongo,
		LocalID: outputresource.LocalIDAzureCosmosDBMongo,
		Type:    outputresource.TypeARM,
		Resource: map[string]string{
			handlers.ManagedKey: "false",

			// Truncate the database part of the ID to make an ID for the account
			handlers.CosmosDBAccountIDKey:    azresources.MakeID(databaseID.SubscriptionID, databaseID.ResourceGroup, databaseID.Types[0]),
			handlers.CosmosDBDatabaseIDKey:   databaseID.ID,
			handlers.CosmosDBAccountNameKey:  databaseID.Types[0].Name,
			handlers.CosmosDBDatabaseNameKey: databaseID.Types[1].Name,
		},
	}
	return []outputresource.OutputResource{resource}, nil
}
