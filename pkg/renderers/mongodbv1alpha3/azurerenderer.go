// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodbv1alpha3

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/radclient"
	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
)

var cosmosAccountDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureCosmosAccount,
}

var _ renderers.Renderer = (*AzureRenderer)(nil)

type AzureRenderer struct {
}

func (r *AzureRenderer) GetDependencyIDs(ctx context.Context, resource renderers.RendererResource) ([]azresources.ResourceID, []azresources.ResourceID, error) {
	return nil, nil, nil
}

func (r AzureRenderer) Render(ctx context.Context, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	properties := radclient.MongoDBResourceProperties{}
	resource := options.Resource
	err := resource.ConvertDefinition(&properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources := []outputresource.OutputResource{}

	results, err := RenderResource(resource.ResourceName, properties)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	resources = append(resources, results...)

	computedValues, secretValues := MakeSecretsAndValues(resource.ResourceName, properties)

	return renderers.RendererOutput{
		Resources:      resources,
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func RenderResource(name string, properties radclient.MongoDBResourceProperties) ([]outputresource.OutputResource, error) {
	if properties.Secrets != nil {
		// When the user-specified secret is present, this is the usecase where the user is running
		// their own custom Redis instance (using a container, or hosted elsewhere).
		//
		// In that case we don't have an OutputResaource, only Computed and Secret values.
		return nil, nil
	}
	if properties.Resource == nil || *properties.Resource == "" {
		return nil, renderers.ErrResourceMissingForResource
	}

	databaseID, err := renderers.ValidateResourceID(*properties.Resource, CosmosMongoResourceType, "CosmosDB Mongo Database")
	if err != nil {
		return nil, err
	}

	// Truncate the database part of the ID to make an ID for the account
	cosmosAccountID := databaseID.Truncate()

	cosmosAccountResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosAccount,
		ResourceKind: resourcekinds.AzureCosmosAccount,
		Resource: map[string]string{
			handlers.CosmosDBAccountIDKey:   cosmosAccountID.ID,
			handlers.CosmosDBAccountNameKey: databaseID.Types[0].Name,
			handlers.CosmosDBAccountKindKey: string(documentdb.DatabaseAccountKindMongoDB),
		},
	}

	databaseResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosDBMongo,
		ResourceKind: resourcekinds.AzureCosmosDBMongo,
		Resource: map[string]string{
			handlers.CosmosDBAccountIDKey:    cosmosAccountID.ID,
			handlers.CosmosDBDatabaseIDKey:   databaseID.ID,
			handlers.CosmosDBAccountNameKey:  databaseID.Types[0].Name,
			handlers.CosmosDBDatabaseNameKey: databaseID.Types[1].Name,
		},
		Dependencies: []outputresource.Dependency{cosmosAccountDependency},
	}
	return []outputresource.OutputResource{cosmosAccountResource, databaseResource}, nil
}

func MakeSecretsAndValues(name string, properties radclient.MongoDBResourceProperties) (map[string]renderers.ComputedValueReference, map[string]renderers.SecretValueReference) {
	computedValues := map[string]renderers.ComputedValueReference{
		DatabaseValue: {
			Value: name,
		},
	}
	if properties.Secrets == nil {
		return computedValues, nil
	}

	// Currently user-specfied secrets are stored in the `secrets` property of the resource, and
	// thus serialized to our database.
	//
	// TODO(#1767): We need to store these in a secret store.
	secretValues := map[string]renderers.SecretValueReference{
		ConnectionStringValue: {Value: *properties.Secrets.ConnectionString},
		UsernameStringValue: {Value: *properties.Secrets.Username},
		PasswordValue: {Value: *properties.Secrets.Password},
	}

	return computedValues, secretValues
}
