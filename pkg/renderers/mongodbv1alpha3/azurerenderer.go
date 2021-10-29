// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

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

var _ renderers.Renderer = (*AzureRenderer)(nil)

type AzureRenderer struct {
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, error) {
	return nil, nil
}

func (r AzureRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := MongoDBComponentProperties{}
	resource := options.Resource
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

	computedValues, secretValues := MakeSecretsAndValues(resource.ResourceName)

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderManaged(name string, properties MongoDBComponentProperties) ([]outputresource.OutputResource, error) {
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
			handlers.CosmosDBAccountKindKey:  string(documentdb.DatabaseAccountKindMongoDB),
		},
	}

	// generate data we can use to manage a cosmosdb instance
	databaseResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosDBMongo,
		ResourceKind: resourcekinds.AzureCosmosDBMongo,
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

func RenderUnmanaged(name string, properties MongoDBComponentProperties) ([]outputresource.OutputResource, error) {
	if properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForUnmanagedResource
	}

	databaseID, err := renderers.ValidateResourceID(properties.Resource, CosmosMongoResourceType, "CosmosDB Mongo Database")
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
			handlers.CosmosDBAccountKindKey: string(documentdb.DatabaseAccountKindMongoDB),
		},
	}

	databaseResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosDBMongo,
		ResourceKind: resourcekinds.AzureCosmosDBMongo,
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

func MakeSecretsAndValues(name string) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValues := map[string]renderers.ComputedValueReference{
		DatabaseValue: {
			Value: name,
		},
	}
	secretValues := map[string]renderers.SecretValueReference{
		ConnectionStringValue: {
			LocalID: cosmosAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
			Action:        "listConnectionStrings",
			ValueSelector: "/connectionStrings/0/connectionString",

			// By-convention the resource type is used as the transformer name.
			Transformer: CosmosMongoResourceType.Type(),
		},
	}

	return computedValues, secretValues
}
