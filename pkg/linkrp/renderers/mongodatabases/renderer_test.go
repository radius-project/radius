/*
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mongodatabases

import (
	"context"
	"testing"

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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
	expectedOutputResources := []rpv1.OutputResource{
		{
			LocalID:       rpv1.LocalIDAzureCosmosAccount,
			ResourceType:  accountResourceType,
			RadiusManaged: to.Ptr(false),
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &accountResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
					APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				},
			},
		},
		{
			LocalID:       rpv1.LocalIDAzureCosmosDBMongo,
			ResourceType:  dbResourceType,
			RadiusManaged: to.Ptr(false),
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &dbResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
					APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				},
			},
			Dependencies: []rpv1.Dependency{
				{
					LocalID: rpv1.LocalIDAzureCosmosAccount,
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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

	expectedSecretValues := map[string]rpv1.SecretValueReference{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.SqlDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeRecipe,
			MongoDatabaseRecipeProperties: datamodel.MongoDatabaseRecipeProperties{
				Recipe: linkrp.LinkRecipe{
					Name: "mongodb",
					Parameters: map[string]any{
						"throughput": 400,
					},
				},
			},
		},
	}

	expectedComputedValues := map[string]renderers.ComputedValueReference{
		renderers.DatabaseNameValue: {
			LocalID:     rpv1.LocalIDAzureCosmosDBMongo,
			JSONPointer: "/properties/resource/id",
		},
	}

	expectedOutputResources := []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
			RadiusManaged:        to.Ptr(true),
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
		},
		{
			LocalID: rpv1.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			RadiusManaged:        to.Ptr(true),
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
			Dependencies:         []rpv1.Dependency{{LocalID: rpv1.LocalIDAzureCosmosAccount}},
		},
	}

	output, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe: linkrp.LinkRecipe{
				Name: "mongodb",
				Parameters: map[string]any{
					"throughput": 400,
				},
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:     linkrp.MongoDatabasesResourceType,
			EnvParameters: map[string]any{
				"name": "account-mongo-db",
			},
		}})
	require.NoError(t, err)
	// Recipe properties
	require.Equal(t, mongoDBResource.Properties.Recipe.Name, output.RecipeData.Name)
	require.Equal(t, mongoDBResource.Properties.Recipe.Parameters, output.RecipeData.Parameters)
	require.Equal(t, "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1", output.RecipeData.TemplatePath)
	require.Equal(t, clientv2.DocumentDBManagementClientAPIVersion, output.RecipeData.APIVersion)

	// Secrets
	require.Equal(t, 1, len(output.SecretValues))
	require.Equal(t, rpv1.LocalIDAzureCosmosAccount, output.SecretValues[renderers.ConnectionStringValue].LocalID)
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
				Type: linkrp.MongoDatabasesResourceType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
			},
			Mode: datamodel.LinkModeRecipe,
			MongoDatabaseRecipeProperties: datamodel.MongoDatabaseRecipeProperties{
				Recipe: linkrp.LinkRecipe{
					Name: "mongodb",
				},
			},
		},
	}

	_, err := renderer.Render(ctx, &mongoDBResource, renderers.RenderOptions{
		RecipeProperties: linkrp.RecipeProperties{
			LinkRecipe: linkrp.LinkRecipe{
				Name: "mongodb",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			LinkType:     linkrp.RedisCachesResourceType,
			EnvParameters: map[string]any{
				"name": "account-mongo-db",
			},
		}})
	require.Error(t, err)
	require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
	require.Equal(t, "link type \"Applications.Link/redisCaches\" of provided recipe \"mongodb\" is incompatible with \"Applications.Link/mongoDatabases\" resource type. Recipe link type must match link resource type.", err.(*v1.ErrClientRP).Message)
}
