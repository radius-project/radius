// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/model"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/mongodatabases"
	corerpDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func buildTestMongoResource() (resourceID resources.ID, testResource datamodel.MongoDatabase, rendererOutput renderers.RendererOutput) {
	id := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.connector/mongodatabases/mongo0"
	resourceID = getResourceID(id)
	testResource = datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   id,
			Name: "mongo0",
			Type: "applications.connector/mongodatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	azureMongoOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDAzureCosmosAccount,
				},
			},
		},
	}

	rendererOutput = renderers.RendererOutput{
		Resources: azureMongoOutputResources,
		SecretValues: map[string]rp.SecretValueReference{
			renderers.ConnectionStringValue: {
				LocalID: outputresource.LocalIDAzureCosmosAccount,
				// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
				Action:        "listConnectionStrings",
				ValueSelector: "/connectionStrings/0/connectionString",
				Transformer: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderAzure,
					Type:     resourcekinds.AzureCosmosDBMongo,
				},
			},
		},
		ComputedValues: map[string]renderers.ComputedValueReference{
			renderers.DatabaseNameValue: {
				Value: "test-database",
			},
		},
	}

	return
}

func buildTestMongoRecipe() (resourceID resources.ID, testResource datamodel.MongoDatabase, rendererOutput renderers.RendererOutput) {
	id := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.connector/mongodatabases/mongo0"
	resourceID = getResourceID(id)
	testResource = datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   id,
			Name: "mongo0",
			Type: "applications.connector/mongodatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Recipe: datamodel.ConnectorRecipe{
					Name: "mongoDB",
					Parameters: map[string]interface{}{
						"ResourceGroup": "testRG",
						"Subscription":  "Radius-Test",
					},
				},
			},
		},
	}

	rendererOutput = renderers.RendererOutput{
		SecretValues: map[string]rp.SecretValueReference{
			renderers.ConnectionStringValue: {
				LocalID: outputresource.LocalIDAzureCosmosAccount,
				// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
				Action:               "listConnectionStrings",
				ValueSelector:        "/connectionStrings/0/connectionString",
				ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
				Transformer: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderAzure,
					Type:     resourcekinds.AzureCosmosDBMongo,
				},
			},
		},
		ComputedValues: map[string]renderers.ComputedValueReference{
			renderers.DatabaseNameValue: {
				LocalID:              outputresource.LocalIDAzureCosmosDBMongo,
				ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
				JSONPointer:          "/properties/resource/id",
			},
		},
		RecipeData: datamodel.RecipeData{
			RecipeProperties: datamodel.RecipeProperties{
				ConnectorRecipe: datamodel.ConnectorRecipe{
					Name:       testResource.Properties.Recipe.Name,
					Parameters: testResource.Properties.Recipe.Parameters,
				},
				TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
			},
			APIVersion: clients.GetAPIVersionFromUserAgent(documentdb.UserAgent()),
			Provider:   resourcemodel.ProviderAzure,
		},
	}

	return
}

func buildTestMongoResourceMixedCaseResourceType() (resourceID resources.ID, testResource datamodel.MongoDatabase, rendererOutput renderers.RendererOutput) {
	id := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.connector/mongodatabases/mongo0"
	resourceID = getResourceID(id)
	testResource = datamodel.MongoDatabase{
		TrackedResource: v1.TrackedResource{
			ID:   id,
			Name: "mongo0",
			Type: "Applications.Connector/MongoDatabases",
		},
		Properties: datamodel.MongoDatabaseProperties{
			MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				},
				Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	azureMongoOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDAzureCosmosAccount,
				},
			},
		},
	}

	rendererOutput = renderers.RendererOutput{
		Resources: azureMongoOutputResources,
		SecretValues: map[string]rp.SecretValueReference{
			renderers.ConnectionStringValue: {
				LocalID: outputresource.LocalIDAzureCosmosAccount,
				// https://docs.microsoft.com/en-us/rest/api/cosmos-db-resource-provider/2021-04-15/database-accounts/list-connection-strings
				Action:        "listConnectionStrings",
				ValueSelector: "/connectionStrings/0/connectionString",
				Transformer: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderAzure,
					Type:     resourcekinds.AzureCosmosDBMongo,
				},
			},
		},
		ComputedValues: map[string]renderers.ComputedValueReference{
			renderers.DatabaseNameValue: {
				Value: "test-database",
			},
		},
	}

	return
}

func buildFetchSecretsInput(withRecipe bool) ResourceData {
	var resourceID resources.ID
	var testResource datamodel.MongoDatabase
	var rendererOutput renderers.RendererOutput
	if withRecipe {
		resourceID, testResource, rendererOutput = buildTestMongoRecipe()
	} else {
		resourceID, testResource, rendererOutput = buildTestMongoResource()
	}
	testResource.Properties.Secrets = datamodel.MongoDatabaseSecrets{
		Username:         "testUser",
		Password:         "testPassword",
		ConnectionString: "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255",
	}

	secretValues := map[string]rp.SecretValueReference{
		renderers.UsernameStringValue:   {Value: "testUser"},
		renderers.PasswordStringHolder:  {Value: "testPassword"},
		renderers.ConnectionStringValue: {Value: "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255"},
	}

	computedValues := map[string]interface{}{
		renderers.DatabaseNameValue: "db",
	}

	testResource.ComputedValues = computedValues
	testResource.SecretValues = secretValues

	return ResourceData{resourceID, testResource, rendererOutput.Resources, computedValues, secretValues, rendererOutput.RecipeData}
}

func buildEnvironmentResource(recipeName string, providers *corerpDatamodel.ProviderProperties) store.Object {
	environment := corerpDatamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
		},
		Properties: corerpDatamodel.EnvironmentProperties{
			Compute: corerpDatamodel.EnvironmentCompute{
				KubernetesCompute: corerpDatamodel.KubernetesComputeProperties{
					Namespace: "radius-test",
				},
			},
		},
	}
	if recipeName != "" {
		environment.Properties.Recipes = map[string]corerpDatamodel.EnvironmentRecipeProperties{
			recipeName: {
				ConnectorType: "Applications.Connector/MongoDatabases",
				TemplatePath:  "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
			},
		}
	}
	if providers != nil {
		environment.Properties.Providers = *providers
	}
	er := store.Object{
		Metadata: store.Metadata{
			ID: environment.ID,
		},
		Data: environment,
	}
	return er
}

type SharedMocks struct {
	model              model.ApplicationModel
	db                 *store.MockStorageClient
	dbProvider         *dataprovider.MockDataStorageProvider
	recipeHandler      *handlers.MockRecipeHandler
	resourceHandler    *handlers.MockResourceHandler
	renderer           *renderers.MockRenderer
	secretsValueClient *rp.MockSecretValueClient
	storageProvider    *dataprovider.MockDataStorageProvider
}

func setup(t *testing.T) SharedMocks {
	ctrl := gomock.NewController(t)

	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockRecipeHandler := handlers.NewMockRecipeHandler(ctrl)

	model := model.NewModel(
		model.RecipeModel{
			RecipeHandler: mockRecipeHandler,
		},
		[]model.RadiusResourceModel{
			{
				ResourceType: mongodatabases.ResourceType,
				Renderer:     mockRenderer,
			},
		},
		[]model.OutputResourceModel{
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosAccount,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
		},
		map[string]bool{
			resourcemodel.ProviderKubernetes: true,
			resourcemodel.ProviderAzure:      true,
		})

	return SharedMocks{
		model:              model,
		db:                 store.NewMockStorageClient(ctrl),
		dbProvider:         dataprovider.NewMockDataStorageProvider(ctrl),
		recipeHandler:      mockRecipeHandler,
		resourceHandler:    mockResourceHandler,
		renderer:           mockRenderer,
		secretsValueClient: rp.NewMockSecretValueClient(ctrl),
	}
}

func getResourceID(id string) resources.ID {
	resourceID, err := resources.ParseResource(id)
	if err != nil {
		panic(err)
	}

	return resourceID
}

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_Render(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	ctrl := gomock.NewController(t)
	mockRecipeHandler := handlers.NewMockRecipeHandler(ctrl)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}
	t.Run("verify render success", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResource()

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render success with recipe", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoRecipe()
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("mongoDB", &corerpDatamodel.ProviderProperties{Azure: corerpDatamodel.ProviderPropertiesAzure{Scope: "/subscriptions/testSub/resourceGroups/testGroup"}})
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, testRendererOutput.RecipeData, rendererOutput.RecipeData)
		require.Equal(t, testRendererOutput.EnvironmentProviders, rendererOutput.EnvironmentProviders)

	})

	t.Run("verify render success with mixedcase resourcetype", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResourceMixedCaseResourceType()

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render error: invalid environment id", func(t *testing.T) {
		id := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.connector/mongodatabases/mongo0"
		resourceID := getResourceID(id)
		resource := datamodel.MongoDatabase{
			TrackedResource: v1.TrackedResource{
				ID:   id,
				Name: "mongo0",
				Type: "Applications.Connector/MongoDatabases",
			},
			Properties: datamodel.MongoDatabaseProperties{
				MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
					BasicResourceProperties: rp.BasicResourceProperties{
						Environment: "invalid-id",
					},
				},
			},
		}

		_, err := dp.Render(ctx, resourceID, &resource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "provided environment id \"invalid-id\" is not a valid id.", err.(*conv.ErrClientRP).Message)
	})

	t.Run("verify render error", func(t *testing.T) {
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render the resource"))
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		resourceID, testResource, _ := buildTestMongoResource()
		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "failed to render the resource", err.Error())
	})

	t.Run("Invalid resource type", func(t *testing.T) {
		testInvalidResourceID := "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.foo/foo/mongo0"
		parsedID := getResourceID(testInvalidResourceID)
		testInvalidResource := datamodel.MongoDatabase{
			TrackedResource: v1.TrackedResource{
				ID:   testInvalidResourceID,
				Name: "Applications.foo/foo",
				Type: "foo",
			},
			Properties: datamodel.MongoDatabaseProperties{
				MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
					BasicResourceProperties: rp.BasicResourceProperties{
						Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
						Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
					},
					Resource: "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
				},
			},
		}

		_, err := dp.Render(ctx, parsedID, &testInvalidResource)
		require.Error(t, err)
		require.Equal(t, "radius resource type 'Applications.foo/foo' is unsupported", err.Error())
	})

	t.Run("Invalid environment type", func(t *testing.T) {
		id := "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.connector/mongodatabases/mongo0"
		resourceID := getResourceID(id)
		resource := datamodel.MongoDatabase{
			TrackedResource: v1.TrackedResource{
				ID:   id,
				Name: "mongo0",
				Type: "Applications.Connector/MongoDatabases",
			},
			Properties: datamodel.MongoDatabaseProperties{
				MongoDatabaseResponseProperties: datamodel.MongoDatabaseResponseProperties{
					BasicResourceProperties: rp.BasicResourceProperties{
						Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/env/test-env",
					},
				},
			},
		}

		_, err := dp.Render(ctx, resourceID, &resource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "provided environment id type \"Applications.Core/env\" is not a valid type.", err.(*conv.ErrClientRP).Message)

	})

	t.Run("Non existing environment", func(t *testing.T) {
		resourceID, testResource, _ := buildTestMongoResource()

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&store.Object{}, &store.ErrNotFound{})

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "environment \"/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0\" does not exist", err.(*conv.ErrClientRP).Message)
	})

	t.Run("Missing output resource provider", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResource()
		testRendererOutput.Resources[0].ResourceType.Provider = ""

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "output resource \"AzureCosmosAccount\" does not have a provider specified", err.Error())
	})

	t.Run("Unsupported output resource provider", func(t *testing.T) {
		resourceID, testResource, _ := buildTestMongoResource()
		rendererOutput := renderers.RendererOutput{
			Resources: []outputresource.OutputResource{
				{
					LocalID: outputresource.LocalIDAzureCosmosAccount,
					ResourceType: resourcemodel.ResourceType{
						Type:     resourcekinds.AzureCosmosAccount,
						Provider: "unknown",
					},
				},
			},
		}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(rendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "provider unknown is not configured. Cannot support resource type azure.cosmosdb.account", err.(*conv.ErrClientRP).Message)
	})

	t.Run("Azure provider unsupported", func(t *testing.T) {
		testModel := model.NewModel(
			model.RecipeModel{
				RecipeHandler: mockRecipeHandler,
			},
			[]model.RadiusResourceModel{
				{
					ResourceType: mongodatabases.ResourceType,
					Renderer:     mocks.renderer,
				},
			},
			[]model.OutputResourceModel{},
			map[string]bool{
				resourcemodel.ProviderKubernetes: true,
				resourcemodel.ProviderAzure:      false,
			},
		)

		mockdp := deploymentProcessor{testModel, mocks.dbProvider, mocks.secretsValueClient, nil}
		resourceID, testResource, testRendererOutput := buildTestMongoResource()

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		_, err := mockdp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "provider azure is not configured. Cannot support resource type azure.cosmosdb.account", err.(*conv.ErrClientRP).Message)
	})
}

func Test_Deploy(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	t.Run("Verify deploy success", func(t *testing.T) {
		expectedCosmosMongoDBIdentity := resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			Data: resourcemodel.ARMIdentity{},
		}

		expectedCosmosMongoAccountIdentity := resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
			Data: resourcemodel.ARMIdentity{},
		}

		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(expectedCosmosMongoAccountIdentity, map[string]string{}, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(expectedCosmosMongoDBIdentity, map[string]string{}, nil)

		resourceID, _, testRendererOutput := buildTestMongoResource()
		deploymentOutput, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(deploymentOutput.Resources))
		require.NotEqual(t, resourcemodel.ResourceIdentity{}, deploymentOutput.Resources[0].Identity)
		require.NotEqual(t, resourcemodel.ResourceIdentity{}, deploymentOutput.Resources[1].Identity)
		require.Equal(t, testRendererOutput.SecretValues, deploymentOutput.SecretValues)
		require.Equal(t, map[string]interface{}{renderers.DatabaseNameValue: testRendererOutput.ComputedValues[renderers.DatabaseNameValue].Value}, deploymentOutput.ComputedValues)
	})

	t.Run("Verify deploy success with recipe", func(t *testing.T) {
		resources := []string{"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
			"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database"}
		resourceData := map[string]interface{}{
			"id":   "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			"name": "test-database",
			"properties": map[string]interface{}{
				"resource": map[string]string{
					"id": "test-database",
				},
			},
			"resourceGroup": "kachawla-test-cs",
			"type":          "Microsoft.DocumentDB/databaseAccounts/mongodbDatabases",
		}
		mocks.recipeHandler.EXPECT().DeployRecipe(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(resources, nil)
		mocks.recipeHandler.EXPECT().GetResource(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(resourceData, nil)

		resourceID, _, testRendererOutput := buildTestMongoRecipe()
		deploymentOutput, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.NoError(t, err)
		require.Equal(t, testRendererOutput.SecretValues, deploymentOutput.SecretValues)
		require.Equal(t, map[string]interface{}{renderers.DatabaseNameValue: "test-database"}, deploymentOutput.ComputedValues)
		require.Equal(t, resources, deploymentOutput.RecipeData.Resources)
	})

	t.Run("Verify deploy failure with recipe", func(t *testing.T) {
		deploymentName := "recipe" + strconv.FormatInt(time.Now().UnixNano(), 10)
		mocks.recipeHandler.EXPECT().DeployRecipe(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]string{}, fmt.Errorf("failed to deploy recipe - %s", deploymentName))

		resourceID, _, testRendererOutput := buildTestMongoRecipe()
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "failed to deploy recipe - "+deploymentName, err.Error())
	})

	t.Run("Verify deploy failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, errors.New("failed to deploy the resource"))

		resourceID, _, testRendererOutput := buildTestMongoResource()
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "failed to deploy the resource", err.Error())
	})

	t.Run("Verify deploy failure - invalid request", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, conv.NewClientErrInvalidRequest("failed to access connected Azure resource"))

		resourceID, _, testRendererOutput := buildTestMongoResource()
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "failed to access connected Azure resource", err.(*conv.ErrClientRP).Message)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		resourceID, _, testRendererOutput := buildTestMongoResource()
		testRendererOutput.Resources[0].Dependencies = []outputresource.Dependency{
			{LocalID: ""},
		}
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "missing localID for outputresource", err.Error())
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		resourceID, _, testRendererOutput := buildTestMongoResource()
		testRendererOutput.Resources[0].ResourceType.Type = "foo"
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: azure, Type: foo' is unsupported", err.Error())
	})

	t.Run("Missing output resource identity", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

		resourceID, _, testRendererOutput := buildTestMongoResource()
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "output resource \"AzureCosmosAccount\" does not have an identity. This is a bug in the handler or renderer", err.Error())
	})
}

func Test_DeployRenderedResources_ComputedValues(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	testResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	testOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDAzureCosmosAccount,
		ResourceType: testResourceType,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &testResourceType,
			Data:         resourcemodel.ARMIdentity{},
		},
		Resource: map[string]interface{}{
			"some-data": "jsonpointer-value",
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-key1": {
				LocalID: outputresource.LocalIDAzureCosmosAccount,
				Value:   "static-value",
			},
			"test-key2": {
				LocalID:           outputresource.LocalIDAzureCosmosAccount,
				PropertyReference: "property-key",
			},
			"test-key3": {
				LocalID:     outputresource.LocalIDAzureCosmosAccount,
				JSONPointer: "/some-data",
			},
		},
	}

	expectedCosmosAccountIdentity := resourcemodel.ResourceIdentity{
		ResourceType: &resourcemodel.ResourceType{
			Type:     resourcekinds.AzureCosmosAccount,
			Provider: resourcemodel.ProviderAzure,
		},
		Data: resourcemodel.ARMIdentity{},
	}
	properties := map[string]string{"property-key": "property-value"}
	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(expectedCosmosAccountIdentity, properties, nil)

	resourceID, _, _ := buildTestMongoResource()
	deploymentOutput, err := dp.Deploy(ctx, resourceID, rendererOutput)
	require.NoError(t, err)

	expected := map[string]interface{}{
		"test-key1": "static-value",
		"test-key2": "property-value",
		"test-key3": "jsonpointer-value",
	}
	require.Equal(t, expected, deploymentOutput.ComputedValues)
}

func Test_Deploy_InvalidComputedValues(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	outputResource := outputresource.OutputResource{
		LocalID:      "test-local-id",
		ResourceType: resourceType,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourceType,
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{outputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-value": {
				LocalID:     "test-local-id",
				JSONPointer: ".ddkfkdk",
			},
		},
	}

	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

	resourceID, _, _ := buildTestMongoResource()
	_, err := dp.Deploy(ctx, resourceID, rendererOutput)
	require.Error(t, err)
	require.Equal(t, "failed to process JSON Pointer \".ddkfkdk\" for resource: JSON pointer must be empty or start with a \"/", err.Error())
}

func Test_Deploy_MissingJsonPointer(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	outputResource := outputresource.OutputResource{
		LocalID:      "test-local-id",
		ResourceType: resourceType,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourceType,
		},
		Resource: map[string]interface{}{
			"some-data": 3,
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{outputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-value": {
				LocalID:     "test-local-id",
				JSONPointer: "/some-other-data", // this key is missing
			},
		},
	}

	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

	resourceID, _, _ := buildTestMongoResource()
	_, err := dp.Deploy(ctx, resourceID, rendererOutput)
	require.Error(t, err)
	require.Equal(t, "failed to process JSON Pointer \"/some-other-data\" for resource: object has no key \"some-other-data\"", err.Error())
}

func Test_Delete(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	testOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: resourcemodel.ProviderAzure,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosAccount,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: resourcemodel.ARMIdentity{},
			},
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: resourcemodel.ProviderAzure,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: resourcemodel.ProviderAzure,
				},
				Data: resourcemodel.ARMIdentity{},
			},
			Dependencies: []outputresource.Dependency{
				{
					LocalID: outputresource.LocalIDAzureCosmosAccount,
				},
			},
		},
	}

	t.Run("Verify delete success", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)

		resourceID, _, _ := buildTestMongoResource()
		resourceData := ResourceData{
			ID:              resourceID,
			OutputResources: testOutputResources,
		}
		err := dp.Delete(ctx, resourceData)
		require.NoError(t, err)
	})

	t.Run("Verify delete success with recipe resources", func(t *testing.T) {
		mocks.recipeHandler.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
		resourceID, _, rendererOutput := buildTestMongoRecipe()
		resourceData := ResourceData{
			ID: resourceID,
			RecipeData: datamodel.RecipeData{
				RecipeProperties: rendererOutput.RecipeData.RecipeProperties,
				APIVersion:       rendererOutput.RecipeData.APIVersion,
				Resources: []string{"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
					"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database"},
			},
		}
		err := dp.Delete(ctx, resourceData)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		resourceID, _, _ := buildTestMongoResource()
		resourceData := ResourceData{
			ID:              resourceID,
			OutputResources: testOutputResources,
		}
		err := dp.Delete(ctx, resourceData)
		require.Error(t, err)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		resourceID, _, _ := buildTestMongoResource()
		outputResources := []outputresource.OutputResource{
			{
				LocalID: outputresource.LocalIDAzureCosmosDBMongo,
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: resourcemodel.ProviderAzure,
				},
				Identity: resourcemodel.ResourceIdentity{
					ResourceType: &resourcemodel.ResourceType{
						Type:     resourcekinds.AzureCosmosDBMongo,
						Provider: resourcemodel.ProviderAzure,
					},
					Data: resourcemodel.ARMIdentity{},
				},
				Dependencies: []outputresource.Dependency{
					{
						LocalID: "",
					},
				},
			},
		}
		resourceData := ResourceData{
			OutputResources: outputResources,
			ID:              resourceID,
		}

		err := dp.Delete(ctx, resourceData)
		require.Error(t, err)
		require.Equal(t, "missing localID for outputresource", err.Error())
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		outputResources := []outputresource.OutputResource{
			{
				LocalID: outputresource.LocalIDAzureCosmosAccount,
				ResourceType: resourcemodel.ResourceType{
					Type:     "foo",
					Provider: resourcemodel.ProviderAzure,
				},
			},
		}
		resourceID, _, _ := buildTestMongoResource()
		resourceData := ResourceData{
			OutputResources: outputResources,
			ID:              resourceID,
		}
		err := dp.Delete(ctx, resourceData)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: azure, Type: foo' is unsupported", err.Error())
	})
}

func Test_FetchSecrets(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	input := buildFetchSecretsInput(false)
	secrets, err := dp.FetchSecrets(ctx, input)
	require.NoError(t, err)
	require.Equal(t, 3, len(secrets))
}

func Test_FetchSecretsWithRecipe(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	input := buildFetchSecretsInput(true)
	secrets, err := dp.FetchSecrets(ctx, input)
	require.NoError(t, err)
	require.Equal(t, 3, len(secrets))
}

func Test_GetEnvironmentMetadata(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	recipeName := "cosmos-recipe"

	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}
	t.Run("successfully get recipe metadata", func(t *testing.T) {
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource(recipeName, &corerpDatamodel.ProviderProperties{Azure: corerpDatamodel.ProviderPropertiesAzure{Scope: "/subscriptions/testSub/resourceGroups/testGroup"}})
		env := er.Metadata.ID
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		envMetadata, err := dp.getEnvironmentMetadata(ctx, env, recipeName)
		require.NoError(t, err)
		require.Equal(t, "Applications.Connector/MongoDatabases", envMetadata.RecipeConnectorType)
		require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", envMetadata.RecipeTemplatePath)

	})

	t.Run("fail to get recipe metadata", func(t *testing.T) {
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("cosmos-test", &corerpDatamodel.ProviderProperties{Azure: corerpDatamodel.ProviderPropertiesAzure{Scope: "/subscriptions/testSub/resourceGroups/testGroup"}})
		env := er.Metadata.ID
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		_, err := dp.getEnvironmentMetadata(ctx, env, recipeName)
		require.Error(t, err)
		require.Equal(t, fmt.Sprintf("recipe with name %q does not exist in the environment %s", recipeName, env), err.Error())
	})
}
