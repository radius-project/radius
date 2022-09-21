// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var cosmosAccountDependency outputresource.Dependency = outputresource.Dependency{
	LocalID: outputresource.LocalIDAzureCosmosAccount,
}

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.MongoDatabase)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	properties := resource.Properties
	secretValues := getProvidedSecretValues(properties)

	_, err := renderers.ValidateApplicationID(properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}
	if resource.Properties.Recipe.Name != "" {
		rendererOutput, err := RenderAzureRecipe(resource, options, secretValues)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		return rendererOutput, nil
	} else if resource.Properties.Resource == "" {
		return renderers.RendererOutput{
			Resources: []outputresource.OutputResource{},
			ComputedValues: map[string]renderers.ComputedValueReference{
				renderers.DatabaseNameValue: {
					Value: resource.Name,
				},
			},
			SecretValues: secretValues,
		}, nil
	} else {
		// Source resource identifier is provided, currently only Azure resources are expected with non empty resource id
		rendererOutput, err := RenderAzureResource(properties, secretValues)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return rendererOutput, nil
	}
}

func RenderAzureRecipe(resource *datamodel.MongoDatabase, options renderers.RenderOptions, secretValues map[string]rp.SecretValueReference) (renderers.RendererOutput, error) {
	properties := resource.Properties
	if options.RecipeConnectorType != resource.ResourceTypeName() {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest("the connector resource type must match the Recipe Connector")
	}
	recipeData := renderers.RecipeData{
		Name:               properties.Recipe.Name,
		RecipeTemplatePath: options.RecipeTemplatePath,
		APIVersion:         clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()),
		AzureResourceType:  AzureCosmosMongoResourceType,
	}
	// Populate connection string reference if a value isn't provided
	if properties.Secrets.IsEmpty() || properties.Secrets.ConnectionString == "" {
		connStringRef := rp.SecretValueReference{
			LocalID: cosmosAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
			Action:        "listConnectionStrings",
			ValueSelector: "/connectionStrings/0/connectionString",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureCosmosDBMongo,
			},
		}
		secretValues[renderers.ConnectionStringValue] = connStringRef
	}
	return renderers.RendererOutput{
		SecretValues: secretValues,
		RecipeData:   recipeData,
	}, nil
}

func RenderAzureResource(properties datamodel.MongoDatabaseProperties, secretValues map[string]rp.SecretValueReference) (renderers.RendererOutput, error) {
	// Validate fully qualified resource identifier of the source resource is supplied for this connector
	cosmosMongoDBID, err := resources.Parse(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}
	// Validate resource type matches the expected Azure Mongo DB resource type
	err = cosmosMongoDBID.ValidateResourceType(AzureCosmosMongoResourceType)
	if err != nil {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest("the 'resource' field must refer to an Azure CosmosDB Mongo Database resource")
	}

	computedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			Value: cosmosMongoDBID.Name(),
		},
	}

	// Populate connection string reference if a value isn't provided
	if properties.Secrets.IsEmpty() || properties.Secrets.ConnectionString == "" {
		connStringRef := rp.SecretValueReference{
			LocalID: cosmosAccountDependency.LocalID,
			// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
			Action:        "listConnectionStrings",
			ValueSelector: "/connectionStrings/0/connectionString",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureCosmosDBMongo,
			},
		}
		secretValues[renderers.ConnectionStringValue] = connStringRef
	}

	// Build output resources
	// Truncate the database part of the ID to get ID for the account
	cosmosMongoAccountID := cosmosMongoDBID.Truncate()
	cosmosAccountResource := outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureCosmosAccount,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosAccount,
			Provider: resourcemodel.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.CosmosDBAccountIDKey:   cosmosMongoAccountID.String(),
			handlers.CosmosDBAccountNameKey: cosmosMongoDBID.TypeSegments()[0].Name,
			handlers.CosmosDBAccountKindKey: string(documentdb.DatabaseAccountKindMongoDB),
		},
	}

	databaseResource := outputresource.OutputResource{
		LocalID: outputresource.LocalIDAzureCosmosDBMongo,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosDBMongo,
			Provider: resourcemodel.ProviderAzure,
		},
		Resource: map[string]string{
			handlers.CosmosDBAccountIDKey:    cosmosMongoAccountID.String(),
			handlers.CosmosDBDatabaseIDKey:   cosmosMongoDBID.String(),
			handlers.CosmosDBAccountNameKey:  cosmosMongoDBID.TypeSegments()[0].Name,
			handlers.CosmosDBDatabaseNameKey: cosmosMongoDBID.TypeSegments()[1].Name,
		},
		Dependencies: []outputresource.Dependency{cosmosAccountDependency},
	}

	return renderers.RendererOutput{
		Resources:      []outputresource.OutputResource{cosmosAccountResource, databaseResource},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func getProvidedSecretValues(properties datamodel.MongoDatabaseProperties) map[string]rp.SecretValueReference {
	secretValues := map[string]rp.SecretValueReference{}
	if !properties.Secrets.IsEmpty() {
		if properties.Secrets.Username != "" {
			secretValues[renderers.UsernameStringValue] = rp.SecretValueReference{Value: properties.Secrets.Username}
		}
		if properties.Secrets.Password != "" {
			secretValues[renderers.PasswordStringHolder] = rp.SecretValueReference{Value: properties.Secrets.Password}
		}
		if properties.Secrets.ConnectionString != "" {
			secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{Value: properties.Secrets.ConnectionString}
		}
	}

	return secretValues
}
