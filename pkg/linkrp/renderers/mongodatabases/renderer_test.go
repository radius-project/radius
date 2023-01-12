// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/stretchr/testify/require"
)

const (
	userName         = "admin"
	password         = "testpassword"
	connectionString = "test-connection-string"
)

func Test_Render_Success(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeResource,
			MongoDatabaseResourceProperties: datamodel.MongoDatabaseResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	accountResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	dbResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosDBMongo,
		Provider: resourcemodel.ProviderAzure,
	}
	expectedOutputResources := []outputresource.OutputResource{
		{
			LocalID:       outputresource.LocalIDAzureCosmosAccount,
			ResourceType:  accountResourceType,
			RadiusManaged: to.BoolPtr(false),
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &accountResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
					APIVersion: clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()),
				},
			},
		},
		{
			LocalID:       outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType:  dbResourceType,
			RadiusManaged: to.BoolPtr(false),
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &dbResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
					APIVersion: clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()),
				},
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDAzureCosmosAccount,
				},
			},
		},
	}
	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			Value: "test-database",
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.Equal(t, expectedOutputResources, output.Resources)
	require.Equal(t, expectedComputedValues, output.ComputedValues)
	require.Equal(t, "/connectionStrings/0/connectionString", output.SecretValues[renderers.ConnectionStringValue].ValueSelector)
	require.Equal(t, "listConnectionStrings", output.SecretValues[renderers.ConnectionStringValue].Action)
}

func Test_Render_UserSpecifiedSecrets(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeValues,
			MongoDatabaseValuesProperties: datamodel.MongoDatabaseValuesProperties{
				Host: "testAccount1.mongo.cosmos.azure.com",
				Port: 1234,
			},
			Secrets: datamodel.MongoDatabaseSecrets{
				Username:         userName,
				Password:         password,
				ConnectionString: connectionString,
			},
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.NoError(t, err)
	require.Len(t, output.Resources, 0)

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			Value: mongoDBResource.Name,
		},
	}
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	expectedSecretValues := map[string]rp.SecretValueReference{
		renderers.ConnectionStringValue: {Value: connectionString},
		renderers.UsernameStringValue:   {Value: userName},
		renderers.PasswordStringHolder:  {Value: password},
	}
	require.Equal(t, expectedSecretValues, output.SecretValues)
}

func Test_Render_InvalidResourceModel(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.SqlDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, "invalid model conversion", err.Error())
}

func Test_Render_InvalidSourceResourceIdentifier(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeResource,
			MongoDatabaseResourceProperties: datamodel.MongoDatabaseResourceProperties{
				Resource: "//subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must be a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidResourceType(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeResource,
			MongoDatabaseResourceProperties: datamodel.MongoDatabaseResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.SomethingElse/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "the 'resource' field must refer to an Azure CosmosDB Mongo Database resource", err.(*v1.ErrClientRP).Message)
}

func Test_Render_InvalidApplicationID(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "invalid-app-id",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeResource,
			MongoDatabaseResourceProperties: datamodel.MongoDatabaseResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "failed to parse application from the property: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
}

func Test_Render_NoResourceSpecified(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeResource,
		},
	}
	expectedErr := &v1.ErrClientRP{
		Code:    "BadRequest",
		Message: "the 'resource' field must be a valid resource id",
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Equal(t, expectedErr, err)
}

func Test_Render_InvalidMode(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: "abcd",
			MongoDatabaseResourceProperties: datamodel.MongoDatabaseResourceProperties{
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "unsupported mode abcd", err.(*v1.ErrClientRP).Message)
}

func Test_Render_Recipe_Success(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeRecipe,
			MongoDatabaseRecipeProperties: datamodel.MongoDatabaseRecipeProperties{
				Recipe: datamodel.LinkRecipe{
					Name: "mongodb",
				},
			},
		},
	}

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			LocalID:     outputresource.LocalIDAzureCosmosDBMongo,
			JSONPointer: "/properties/resource/id",
		},
	}

	expectedOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
			RadiusManaged:        to.BoolPtr(true),
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			RadiusManaged:        to.BoolPtr(true),
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
			Dependencies:         []outputresource.Dependency{{LocalID: outputresource.LocalIDAzureCosmosAccount}},
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{
		RecipeProperties: datamodel.RecipeProperties{
			LinkRecipe: datamodel.LinkRecipe{
				Name: "mongodb",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:     ResourceType,
		}})
	require.NoError(t, err)
	// Recipe properties
	require.Equal(t, mongoDBResource.Properties.Recipe.Name, output.RecipeData.Name)
	require.Equal(t, "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1", output.RecipeData.TemplatePath)
	require.Equal(t, clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()), output.RecipeData.APIVersion)

	// Secrets
	require.Equal(t, 1, len(output.SecretValues))
	require.Equal(t, outputresource.LocalIDAzureCosmosAccount, output.SecretValues[renderers.ConnectionStringValue].LocalID)
	require.Equal(t, "/connectionStrings/0/connectionString", output.SecretValues[renderers.ConnectionStringValue].ValueSelector)
	require.Equal(t, "listConnectionStrings", output.SecretValues[renderers.ConnectionStringValue].Action)

	// Computed Values
	require.Equal(t, expectedComputedValues, output.ComputedValues)

	// Output resources
	require.Equal(t, 2, len(output.Resources))
	require.Equal(t, expectedOutputResources, output.Resources)
}

func Test_Render_Recipe_InvalidLinkType(t *testing.T) {
	ctx := context.Background()
	renderer := Renderer{}

	mongoDBResource := datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/mongoDatabases/mongo0",
				Name: "mongo0",
				Type: "Applications.Link/mongoDatabases",
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rp.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeRecipe,
			MongoDatabaseRecipeProperties: datamodel.MongoDatabaseRecipeProperties{
				Recipe: datamodel.LinkRecipe{
					Name: "mongodb",
				},
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{
		RecipeProperties: datamodel.RecipeProperties{
			LinkRecipe: datamodel.LinkRecipe{
				Name: "mongodb",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:     "Applications.Link/redisCaches",
		}})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "link type \"Applications.Link/redisCaches\" of provided recipe \"mongodb\" is incompatible with \"Applications.Link/mongoDatabases\" resource type. Recipe link type must match link resource type.", err.(*v1.ErrClientRP).Message)
}
