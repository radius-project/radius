// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdbsqlv1alpha3

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
)

var cosmosAccountDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureCosmosAccount,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r *Renderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r Renderer) Render(ctx context.Context, resource renderers.RendererResource, dependencies map[string]renderers.RendererDependency) (renderers.RendererOutput, error) {
	properties := CosmosDBSQLComponentProperties{}
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources := []outputresource.OutputResource{}
	if properties.Managed {
		results, err := RenderManaged(resource.ResourceName, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		resources = append(resources, results...)
	} else {
		results, err := RenderUnmanaged(resource.ResourceName, properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		resources = append(resources, results...)
	}

	computedValues := map[string]renderers.ComputedValueReference{
		"database": {
			Value: resource.ResourceName,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{
		ConnectionStringValue: {
			LocalID: cosmosAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
			Action:        "listConnectionStrings",
			ValueSelector: "/connectionStrings/0/connectionString",
		},
	}

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderManaged(name string, properties CosmosDBSQLComponentProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource != "" {
		return nil, renderers.ErrResourceSpecifiedForManagedResource
	}

	cosmosAccountResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosAccount,
		ResourceKind: resourcekinds.AzureCosmosAccount,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:              "true",
			handlers.CosmosDBAccountBaseName: name,
			handlers.CosmosDBAccountKindKey:  string(documentdb.DatabaseAccountKindGlobalDocumentDB),
		},
	}

	// generate data we can use to manage a cosmosdb instance
	databaseResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosDBSQL,
		ResourceKind: resourcekinds.AzureCosmosDBSQL,
		Managed:      true,
		Resource: map[string]string{
			handlers.ManagedKey:              "true",
			handlers.CosmosDBAccountBaseName: name,
			handlers.CosmosDBDatabaseNameKey: name,
		},
		Dependencies: []outputresource.Dependency{cosmosAccountDependency},
	}

	return []outputresource.OutputResource{cosmosAccountResource, databaseResource}, nil
}

func RenderUnmanaged(name string, properties CosmosDBSQLComponentProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForUnmanagedResource
	}

	databaseID, err := renderers.ValidateResourceID(properties.Resource, SQLResourceType, "CosmosDB SQL Database")
	if err != nil {
		return nil, err
	}

	// Truncate the database part of the ID to make an ID for the account
	cosmosAccountID := databaseID.Truncate()

	cosmosAccountResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosAccount,
		ResourceKind: resourcekinds.AzureCosmosAccount,
		Resource: map[string]string{
			handlers.ManagedKey:             "false",
			handlers.CosmosDBAccountIDKey:   cosmosAccountID.ID,
			handlers.CosmosDBAccountNameKey: databaseID.Types[0].Name,
			handlers.CosmosDBAccountKindKey: string(documentdb.DatabaseAccountKindGlobalDocumentDB),
		},
	}

	databaseResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosDBSQL,
		ResourceKind: resourcekinds.AzureCosmosDBSQL,
		Resource: map[string]string{
			handlers.ManagedKey:              "false",
			handlers.CosmosDBAccountIDKey:    cosmosAccountID.ID,
			handlers.CosmosDBDatabaseIDKey:   databaseID.ID,
			handlers.CosmosDBAccountNameKey:  databaseID.Types[0].Name,
			handlers.CosmosDBDatabaseNameKey: databaseID.Types[1].Name,
		},
		Dependencies: []outputresource.Dependency{cosmosAccountDependency},
	}
	return []outputresource.OutputResource{cosmosAccountResource, databaseResource}, nil
}
