// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"fmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ renderers.Renderer = (*Renderer)(nil)

type Renderer struct {
}

func (r Renderer) Render(ctx context.Context, dm v1.ResourceDataModel, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	resource, ok := dm.(*datamodel.MongoDatabase)
	if !ok {
		return renderers.RendererOutput{}, v1.ErrInvalidModelConversion
	}

	_, err := renderers.ValidateApplicationID(resource.Properties.Application)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	switch resource.Properties.Mode {
	case datamodel.LinkModeRecipe:
		rendererOutput, err := RenderAzureRecipe(resource, options)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		return rendererOutput, nil
	case datamodel.LinkModeResource:
		// Source resource identifier is provided
		// Currently only Azure resources are supported with non empty resource id
		rendererOutput, err := RenderAzureResource(resource.Properties)
		if err != nil {
			return renderers.RendererOutput{}, err
		}
		return rendererOutput, nil
	case datamodel.LinkModeValues:
		return renderers.RendererOutput{
			Resources: []rpv1.OutputResource{},
			ComputedValues: map[string]renderers.ComputedValueReference{
				renderers.DatabaseNameValue: {
					Value: resource.Name,
				},
			},
			SecretValues: getProvidedSecretValues(resource.Properties),
		}, nil
	default:
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest(fmt.Sprintf("unsupported mode %s", resource.Properties.Mode))
	}
}

func RenderAzureRecipe(resource *datamodel.MongoDatabase, options renderers.RenderOptions) (renderers.RendererOutput, error) {
	err := renderers.ValidateLinkType(resource, options)
	if err != nil {
		return renderers.RendererOutput{}, err
	}

	recipeData := linkrp.RecipeData{
		RecipeProperties: options.RecipeProperties,
		APIVersion:       clientv2.DocumentDBManagementClientAPIVersion,
	}

	secretValues := buildSecretValueReferenceForAzure(resource.Properties)

	computedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			LocalID:     rpv1.LocalIDAzureCosmosDBMongo,
			JSONPointer: "/properties/resource/id", // response of "az resource show" for cosmos mongodb resource contains database name in this property
		},
	}

	// Build expected output resources
	expectedCosmosAccount := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCosmosAccount,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosAccount,
			Provider: resourcemodel.ProviderAzure,
		},
		ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
		RadiusManaged:        to.Ptr(false),
	}

	expectedMongoDBResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCosmosDBMongo,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosDBMongo,
			Provider: resourcemodel.ProviderAzure,
		},
		ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
		RadiusManaged:        to.Ptr(false),
		Dependencies:         []rpv1.Dependency{{LocalID: rpv1.LocalIDAzureCosmosAccount}},
	}

	return renderers.RendererOutput{
		ComputedValues:       computedValues,
		SecretValues:         secretValues,
		Resources:            []rpv1.OutputResource{expectedCosmosAccount, expectedMongoDBResource},
		RecipeData:           recipeData,
		EnvironmentProviders: options.EnvironmentProviders,
	}, nil
}

func RenderAzureResource(properties datamodel.MongoDatabaseProperties) (renderers.RendererOutput, error) {
	// Validate fully qualified resource identifier of the source resource is supplied for this link
	cosmosMongoDBID, err := resources.ParseResource(properties.Resource)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must be a valid resource id")
	}
	// Validate resource type matches the expected Azure Mongo DB resource type
	err = cosmosMongoDBID.ValidateResourceType(AzureCosmosMongoResourceType)
	if err != nil {
		return renderers.RendererOutput{}, v1.NewClientErrInvalidRequest("the 'resource' field must refer to an Azure CosmosDB Mongo Database resource")
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
	cosmosAccountResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCosmosAccount,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosAccount,
			Provider: resourcemodel.ProviderAzure,
		},
		RadiusManaged: to.Ptr(false),
	}
	cosmosAccountResource.Identity = resourcemodel.NewARMIdentity(&cosmosAccountResource.ResourceType, cosmosMongoAccountID.String(), clientv2.DocumentDBManagementClientAPIVersion)

	databaseResource := rpv1.OutputResource{
		LocalID: rpv1.LocalIDAzureCosmosDBMongo,
		ResourceType: resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosDBMongo,
			Provider: resourcemodel.ProviderAzure,
		},
		RadiusManaged: to.Ptr(false),
		Dependencies: []rpv1.Dependency{
			{
				LocalID: rpv1.LocalIDAzureCosmosAccount,
			},
		},
	}
	databaseResource.Identity = resourcemodel.NewARMIdentity(&databaseResource.ResourceType, cosmosMongoDBID.String(), clientv2.DocumentDBManagementClientAPIVersion)

	return renderers.RendererOutput{
		Resources:      []rpv1.OutputResource{cosmosAccountResource, databaseResource},
		ComputedValues: computedValues,
		SecretValues:   secretValues,
	}, nil
}

func getProvidedSecretValues(properties datamodel.MongoDatabaseProperties) map[string]rpv1.SecretValueReference {
	secretValues := map[string]rpv1.SecretValueReference{}
	if !properties.Secrets.IsEmpty() {
		if properties.Secrets.Username != "" {
			secretValues[renderers.UsernameStringValue] = rpv1.SecretValueReference{Value: properties.Secrets.Username}
		}
		if properties.Secrets.Password != "" {
			secretValues[renderers.PasswordStringHolder] = rpv1.SecretValueReference{Value: properties.Secrets.Password}
		}
		if properties.Secrets.ConnectionString != "" {
			secretValues[renderers.ConnectionStringValue] = rpv1.SecretValueReference{Value: properties.Secrets.ConnectionString}
		}
	}

	return secretValues
}

func buildSecretValueReferenceForAzure(properties datamodel.MongoDatabaseProperties) map[string]rpv1.SecretValueReference {
	secretValues := getProvidedSecretValues(properties)

	// Populate connection string reference if a value isn't provided
	_, ok := secretValues[renderers.ConnectionStringValue]
	if !ok {
		secretValues[renderers.ConnectionStringValue] = rpv1.SecretValueReference{
			LocalID:       rpv1.LocalIDAzureCosmosAccount,
			Action:        "listConnectionStrings", // https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
			ValueSelector: "/connectionStrings/0/connectionString",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureCosmosDBMongo,
			},
		}
	}
	return secretValues
}
