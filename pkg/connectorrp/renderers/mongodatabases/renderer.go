// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/azresources"
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

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm conv.DataModelInterface, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.MongoDatabase)
	if !ok {
		return renderers.RendererOutput{}, conv.ErrInvalidModelConversion
	}

	_, err := renderers.ValidateApplicationID(resource.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	if resource.Properties.Recipe.Name != "" {
		rendererOutput, err := RenderAzureRecipe(resource, options)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return rendererOutput, nil
	} else if resource.Properties.Resource != "" {
		// Source resource identifier is provided
		// Currently only Azure resources are supported with non empty resource id
		rendererOutput, err := RenderAzureResource(resource.Properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}

		return rendererOutput, nil
	} else {
		return renderers.RendererOutput{
			Resources: []outputresource.OutputResource{},
			ComputedValues: map[string]renderers.ComputedValueReference{
				renderers.DatabaseNameValue: {
					Value: resource.Name,
				},
			},
			SecretValues: getProvidedSecretValues(resource.Properties),
		}, nil
	}
}

func RenderAzureRecipe(resource *datamodel.MongoDatabase, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	if options.RecipeProperties.ConnectorType != resource.ResourceTypeName() {
		return renderers.RendererOutput{}, conv.NewClientErrInvalidRequest(fmt.Sprintf("connector type %q of provided recipe %q is incompatible with %q resource type. Recipe connector type must match connector resource type.",
			options.RecipeProperties.ConnectorType, options.RecipeProperties.Name, ResourceType))
	}

	recipeData := datamodel.RecipeData{
		Provider:         resourcemodel.ProviderAzure,
		RecipeProperties: options.RecipeProperties,
		APIVersion:       clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()),
	}

	secretValues := buildSecretValueReferenceForAzure(resource.Properties)

	computedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			LocalID:              outputresource.LocalIDAzureCosmosDBMongo,
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
			JSONPointer:          "/properties/resource/id", // response of "az resource show" for cosmos mongodb resource contains database name in this property
		},
	}

	return renderers.RendererOutput{
		ComputedValues: computedValues,
		SecretValues:   secretValues,
		RecipeData:     recipeData,
	}, nil
}

func RenderAzureResource(properties datamodel.MongoDatabaseProperties) (renderers.RendererOutput, error) {
	// Validate fully qualified resource identifier of the source resource is supplied for this connector
	cosmosMongoDBID, err := resources.ParseResource(properties.Resource)
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

	secretValues := buildSecretValueReferenceForAzure(properties)

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
		Dependencies: []outputresource.Dependency{
			{
				LocalID: outputresource.LocalIDAzureCosmosAccount,
			},
		},
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

func buildSecretValueReferenceForAzure(properties datamodel.MongoDatabaseProperties) map[string]rp.SecretValueReference {
	secretValues := getProvidedSecretValues(properties)

	// Populate connection string reference if a value isn't provided
	_, ok := secretValues[renderers.ConnectionStringValue]
	if !ok {
		secretValues[renderers.ConnectionStringValue] = rp.SecretValueReference{
			LocalID:              outputresource.LocalIDAzureCosmosAccount,
			Action:               "listConnectionStrings", // https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
			ValueSelector:        "/connectionStrings/0/connectionString",
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureCosmosDBMongo,
			},
		}
	}

	return secretValues
}
