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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	corerp_dm "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

const (
	applicationID = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication"
	envID         = "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0"

	mongoLinkName = "mongo0"
	mongoLinkID   = "/subscriptions/testSub/resourceGroups/testGroup/providers/applications.link/mongodatabases/mongo0"
	mongoLinkType = "applications.link/mongodatabases"

	cosmosAccountID        = "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account"
	cosmosMongoID          = "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database"
	cosmosConnectionString = "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255"

	daprLinkName        = "test-state-store"
	daprLinkID          = "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStores/test-state-store"
	azureTableStorageID = "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable"
	redisId             = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"
	recipeName          = "testRecipe"

	modeRecipe   = "recipe"
	modeResource = "resource"
	modeValues   = "values"
)

var (
	mongoLinkResourceID = getResourceID(mongoLinkID)
	redisLinkResourceId = getResourceID(redisId)
	recipeParams        = map[string]any{
		"throughput": 400,
	}
)

func buildInputResourceMongo(mode string) (testResource datamodel.MongoDatabase) {
	testResource = datamodel.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   mongoLinkID,
				Name: mongoLinkName,
				Type: mongoLinkType,
			},
		},
		Properties: datamodel.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: applicationID,
				Environment: envID,
			},
		},
	}

	if mode == modeResource {
		testResource.Properties.Resource = cosmosMongoID
	} else if mode == modeRecipe {
		testResource.Properties.Recipe = datamodel.LinkRecipe{
			Name:       recipeName,
			Parameters: recipeParams,
		}
	} else if mode == modeValues {
		testResource.Properties.Secrets = datamodel.MongoDatabaseSecrets{
			Username:         "testUser",
			Password:         "testPassword",
			ConnectionString: cosmosConnectionString,
		}
	}

	return
}

func buildOutputResourcesMongo(mode string) []rpv1.OutputResource {
	radiusManaged := false
	if mode == modeRecipe {
		radiusManaged = true
	}

	accountResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	dbResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosDBMongo,
		Provider: resourcemodel.ProviderAzure,
	}

	return []rpv1.OutputResource{
		{
			LocalID:              rpv1.LocalIDAzureCosmosAccount,
			ResourceType:         accountResourceType,
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &accountResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         cosmosAccountID,
					APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				},
			},
			RadiusManaged: &radiusManaged,
		},
		{
			LocalID:              rpv1.LocalIDAzureCosmosDBMongo,
			ResourceType:         dbResourceType,
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &dbResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         cosmosMongoID,
					APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				},
			},
			Resource: map[string]any{
				"properties": map[string]any{
					"resource": map[string]string{
						"id": "test-database",
					},
				},
			},
			RadiusManaged: &radiusManaged,
			Dependencies:  []rpv1.Dependency{{LocalID: rpv1.LocalIDAzureCosmosAccount}},
		},
	}
}

func buildRendererOutputRedis() (rendererOutput renderers.RendererOutput) {
	computedValues := map[string]renderers.ComputedValueReference{
		renderers.Host: {
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/hostName",
		},
		renderers.Port: {
			LocalID:     rpv1.LocalIDAzureRedis,
			JSONPointer: "/properties/sslPort",
		},
	}
	secretValues := map[string]rpv1.SecretValueReference{renderers.PasswordStringHolder: {
		LocalID:       rpv1.LocalIDAzureRedis,
		Action:        "listKeys",
		ValueSelector: "/primaryKey",
	},
		renderers.ConnectionStringValue: {
			LocalID:       rpv1.LocalIDAzureRedis,
			Action:        "listKeys",
			ValueSelector: "/primaryKey",
			Transformer: resourcemodel.ResourceType{
				Provider: resourcemodel.ProviderAzure,
				Type:     resourcekinds.AzureRedis,
			},
		},
	}
	recipeData := datamodel.RecipeData{
		RecipeProperties: datamodel.RecipeProperties{
			LinkRecipe: datamodel.LinkRecipe{
				Name: "redis",
			},
			TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/redis:v1",
		},
		APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
		Provider:   resourcemodel.ProviderAzure,
	}
	rendererOutput = renderers.RendererOutput{
		SecretValues:   secretValues,
		ComputedValues: computedValues,
		RecipeData:     recipeData,
	}

	return

}
func buildRendererOutputMongo(mode string) (rendererOutput renderers.RendererOutput) {
	computedValues := map[string]renderers.ComputedValueReference{}
	secretValues := map[string]rpv1.SecretValueReference{}
	if mode == modeResource || mode == modeRecipe {
		computedValues = map[string]renderers.ComputedValueReference{
			renderers.DatabaseNameValue: {
				LocalID:     rpv1.LocalIDAzureCosmosDBMongo,
				JSONPointer: "/properties/resource/id",
			},
			renderers.Host: {
				Value: 8080,
			},
		}

		secretValues = map[string]rpv1.SecretValueReference{
			renderers.ConnectionStringValue: {
				LocalID:       rpv1.LocalIDAzureCosmosAccount,
				Action:        "listConnectionStrings",
				ValueSelector: "/connectionStrings/0/connectionString",
				Transformer: resourcemodel.ResourceType{
					Provider: resourcemodel.ProviderAzure,
					Type:     resourcekinds.AzureCosmosDBMongo,
				},
			},
		}
	} else if mode == modeValues {
		computedValues = map[string]renderers.ComputedValueReference{
			renderers.DatabaseNameValue: {
				Value: mongoLinkName,
			},
		}

		secretValues = map[string]rpv1.SecretValueReference{
			renderers.UsernameStringValue:   {Value: "testUser"},
			renderers.PasswordStringHolder:  {Value: "testPassword"},
			renderers.ConnectionStringValue: {Value: cosmosConnectionString},
		}
	}

	recipeData := datamodel.RecipeData{}
	if mode == modeRecipe {
		recipeData = datamodel.RecipeData{
			RecipeProperties: datamodel.RecipeProperties{
				LinkRecipe: datamodel.LinkRecipe{
					Name:       recipeName,
					Parameters: recipeParams,
				},
				TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
				EnvParameters: map[string]any{
					"name": "account-mongo-db",
				},
			},
			APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
			Provider:   resourcemodel.ProviderAzure,
		}
	}

	rendererOutput = renderers.RendererOutput{
		Resources:      buildOutputResourcesMongo(mode),
		SecretValues:   secretValues,
		ComputedValues: computedValues,
		RecipeData:     recipeData,
	}

	return
}

func buildOutputResourcesDapr(mode string) []rpv1.OutputResource {
	radiusManaged := true

	return []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDDaprStateStoreAzureStorage,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.DaprStateStoreAzureStorage,
				Provider: resourcemodel.ProviderAzure,
			},
			Resource: map[string]string{
				handlers.KubernetesNameKey:       daprLinkName,
				handlers.KubernetesNamespaceKey:  "radius-test",
				handlers.ApplicationName:         "testApplication",
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ResourceName:            daprLinkName,
			},
			RadiusManaged: &radiusManaged,
		},
	}
}

func buildApplicationResource(namespace string) *store.Object {
	if namespace == "" {
		namespace = "radius-test-app"
	}

	app := corerp_dm.Application{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: applicationID,
			},
		},
		Properties: corerp_dm.ApplicationProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Status: rpv1.ResourceStatus{
					Compute: &rpv1.EnvironmentCompute{
						Kind: rpv1.KubernetesComputeKind,
						KubernetesCompute: rpv1.KubernetesComputeProperties{
							Namespace: namespace,
						},
					},
				},
			},
		},
	}

	return &store.Object{
		Metadata: store.Metadata{
			ID: app.ID,
		},
		Data: app,
	}
}

func buildEnvironmentResource(recipeName string, providers *corerp_dm.Providers) *store.Object {
	environment := corerp_dm.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
		},
		Properties: corerp_dm.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				KubernetesCompute: rpv1.KubernetesComputeProperties{
					Namespace: "radius-test-env",
				},
			},
		},
	}
	if recipeName != "" {
		environment.Properties.Recipes = map[string]corerp_dm.EnvironmentRecipeProperties{
			recipeName: {
				LinkType:     "Applications.Link/MongoDatabases",
				TemplatePath: "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb",
			},
		}
	}
	if providers != nil {
		environment.Properties.Providers = *providers
	}

	return &store.Object{
		Metadata: store.Metadata{
			ID: environment.ID,
		},
		Data: environment,
	}
}

type SharedMocks struct {
	model              model.ApplicationModel
	db                 *store.MockStorageClient
	dbProvider         *dataprovider.MockDataStorageProvider
	recipeHandler      *handlers.MockRecipeHandler
	resourceHandler    *handlers.MockResourceHandler
	renderer           *renderers.MockRenderer
	secretsValueClient *sv.MockSecretValueClient
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
				ResourceType: linkrp.MongoDatabasesResourceType,
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
				ResourceHandler:        mockResourceHandler,
				SecretValueTransformer: &mongodatabases.AzureTransformer{},
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.DaprStateStoreAzureStorage,
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
		secretsValueClient: sv.NewMockSecretValueClient(ctrl),
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
	logger, err := ucplog.NewTestLogger(t)
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
		testResource := buildInputResourceMongo(modeResource)
		testRendererOutput := buildRendererOutputMongo(modeResource)
		app, err := resources.ParseResource(testResource.Properties.Application)
		require.NoError(t, err)
		env, err := resources.ParseResource(testResource.Properties.Environment)
		require.NoError(t, err)
		testRendererOutput.RecipeContext = datamodel.RecipeContext{
			Resource: datamodel.Resource{
				ResourceInfo: datamodel.ResourceInfo{
					ID:   testResource.ID,
					Name: testResource.Name,
				},
				Type: testResource.Type,
			},
			Application: datamodel.ResourceInfo{
				ID:   testResource.Properties.Application,
				Name: app.Name(),
			},
			Environment: datamodel.ResourceInfo{
				ID:   testResource.Properties.Environment,
				Name: env.Name(),
			},
			Runtime: datamodel.Runtime{
				Kubernetes: datamodel.Kubernetes{
					Namespace:            "radius-test-app",
					EnvironmentNamespace: "radius-test-env",
				},
			},
		}
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildEnvironmentResource("", nil), nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		rendererOutput, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
		require.Equal(t, testRendererOutput.ComputedValues, rendererOutput.ComputedValues)
		require.Equal(t, testRendererOutput.SecretValues, rendererOutput.SecretValues)
		require.Equal(t, testRendererOutput.RecipeContext.Resource, rendererOutput.RecipeContext.Resource)
		require.Equal(t, testRendererOutput.RecipeContext.Application, rendererOutput.RecipeContext.Application)
		require.Equal(t, testRendererOutput.RecipeContext.Environment, rendererOutput.RecipeContext.Environment)
		require.Equal(t, testRendererOutput.RecipeContext.Runtime, rendererOutput.RecipeContext.Runtime)
	})

	t.Run("verify render success with environment scoped link", func(t *testing.T) {
		testResource := buildInputResourceMongo(modeResource)
		testResource.Properties.Application = ""
		testRendererOutput := buildRendererOutputMongo(modeResource)
		env, err := resources.ParseResource(testResource.Properties.Environment)
		require.NoError(t, err)
		testRendererOutput.RecipeContext = datamodel.RecipeContext{
			Resource: datamodel.Resource{
				ResourceInfo: datamodel.ResourceInfo{
					ID:   testResource.ID,
					Name: testResource.Name,
				},
				Type: testResource.Type,
			},
			Environment: datamodel.ResourceInfo{
				ID:   testResource.Properties.Environment,
				Name: env.Name(),
			},
			Runtime: datamodel.Runtime{
				Kubernetes: datamodel.Kubernetes{
					Namespace:            "radius-test-env",
					EnvironmentNamespace: "radius-test-env",
				},
			},
		}
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildEnvironmentResource("", nil), nil)

		rendererOutput, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
		require.Equal(t, testRendererOutput.ComputedValues, rendererOutput.ComputedValues)
		require.Equal(t, testRendererOutput.SecretValues, rendererOutput.SecretValues)
		require.Equal(t, testRendererOutput.RecipeContext.Resource, rendererOutput.RecipeContext.Resource)
		require.Equal(t, testRendererOutput.RecipeContext.Application, rendererOutput.RecipeContext.Application)
		require.Equal(t, testRendererOutput.RecipeContext.Environment, rendererOutput.RecipeContext.Environment)
		require.Equal(t, testRendererOutput.RecipeContext.Runtime, rendererOutput.RecipeContext.Runtime)
	})

	t.Run("verify render success with recipe", func(t *testing.T) {
		testResource := buildInputResourceMongo(modeRecipe)
		testRendererOutput := buildRendererOutputMongo(modeRecipe)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		er := buildEnvironmentResource(recipeName, &corerp_dm.Providers{Azure: corerp_dm.ProvidersAzure{Scope: "/subscriptions/testSub/resourceGroups/testGroup"}})
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		rendererOutput, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, testRendererOutput.Resources, rendererOutput.Resources)
		require.Equal(t, testRendererOutput.ComputedValues, rendererOutput.ComputedValues)
		require.Equal(t, testRendererOutput.SecretValues, rendererOutput.SecretValues)
		require.Equal(t, testRendererOutput.RecipeData, rendererOutput.RecipeData)
		require.Equal(t, testRendererOutput.EnvironmentProviders, rendererOutput.EnvironmentProviders)
	})

	t.Run("verify render success with mixedcase resourcetype", func(t *testing.T) {
		testResource := buildInputResourceMongo(modeResource)
		testResource.Type = "Applications.Link/MongoDatabases"
		testRendererOutput := buildRendererOutputMongo(modeResource)

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		rendererOutput, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render error: invalid environment id", func(t *testing.T) {
		resource := datamodel.MongoDatabase{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   mongoLinkID,
					Name: mongoLinkName,
					Type: mongoLinkType,
				},
			},
			Properties: datamodel.MongoDatabaseProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "invalid-id",
				},
			},
		}

		_, err := dp.Render(ctx, mongoLinkResourceID, &resource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "invalid-id is not a valid resource id for Applications.Core/environments.", err.(*v1.ErrClientRP).Message)
	})

	t.Run("verify render error", func(t *testing.T) {
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render the resource"))
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		testResource := buildInputResourceMongo(modeResource)

		_, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "failed to render the resource", err.Error())
	})

	t.Run("Invalid resource type", func(t *testing.T) {
		testInvalidResourceID := "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.foo/foo/mongo0"
		parsedID := getResourceID(testInvalidResourceID)
		testInvalidResource := datamodel.MongoDatabase{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   testInvalidResourceID,
					Name: "Applications.foo/foo",
					Type: "foo",
				},
			},
			Properties: datamodel.MongoDatabaseProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: applicationID,
					Environment: envID,
				},
				MongoDatabaseResourceProperties: datamodel.MongoDatabaseResourceProperties{
					Resource: cosmosMongoID,
				},
			},
		}

		_, err := dp.Render(ctx, parsedID, &testInvalidResource)
		require.Error(t, err)
		require.Equal(t, "radius resource type 'Applications.foo/foo' is unsupported", err.Error())
	})

	t.Run("Invalid environment type", func(t *testing.T) {
		resource := datamodel.MongoDatabase{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID:   mongoLinkID,
					Name: mongoLinkName,
					Type: mongoLinkType,
				},
			},
			Properties: datamodel.MongoDatabaseProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/env/test-env",
				},
			},
		}

		_, err := dp.Render(ctx, mongoLinkResourceID, &resource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "linked \"/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/env/test-env\" has invalid Applications.Core/environments resource type.", err.(*v1.ErrClientRP).Message)

	})

	t.Run("Non existing environment", func(t *testing.T) {
		testResource := buildInputResourceMongo(modeResource)

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&store.Object{}, &store.ErrNotFound{})

		_, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "linked resource /subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0 does not exist", err.(*v1.ErrClientRP).Message)
	})

	t.Run("Missing output resource provider", func(t *testing.T) {
		testResource := buildInputResourceMongo(modeResource)
		testRendererOutput := buildRendererOutputMongo(modeResource)
		testRendererOutput.Resources[0].ResourceType.Provider = ""

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		_, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "output resource \"AzureCosmosAccount\" does not have a provider specified", err.Error())
	})

	t.Run("Unsupported output resource provider", func(t *testing.T) {
		testResource := buildInputResourceMongo(modeResource)
		rendererOutput := renderers.RendererOutput{
			Resources: []rpv1.OutputResource{
				{
					LocalID: rpv1.LocalIDAzureCosmosAccount,
					ResourceType: resourcemodel.ResourceType{
						Type:     resourcekinds.AzureCosmosAccount,
						Provider: "unknown",
					},
				},
			},
		}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(rendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		_, err := dp.Render(ctx, mongoLinkResourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "provider unknown is not configured. Cannot support resource type azure.cosmosdb.account", err.(*v1.ErrClientRP).Message)
	})

	t.Run("Azure provider unsupported", func(t *testing.T) {
		testModel := model.NewModel(
			model.RecipeModel{
				RecipeHandler: mockRecipeHandler,
			},
			[]model.RadiusResourceModel{
				{
					ResourceType: linkrp.MongoDatabasesResourceType,
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
		testResource := buildInputResourceMongo(modeResource)
		testRendererOutput := buildRendererOutputMongo(modeResource)

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(2).Return(mocks.db, nil)
		er := buildEnvironmentResource("", nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(buildApplicationResource(""), nil)

		_, err := mockdp.Render(ctx, mongoLinkResourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "provider azure is not configured. Cannot support resource type azure.cosmosdb.account", err.(*v1.ErrClientRP).Message)
	})
}

func Test_Deploy(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	t.Run("Verify deploy success", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(2).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

		testRendererOutput := buildRendererOutputMongo(modeResource)

		deploymentOutput, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(deploymentOutput.Resources))
		require.NotEqual(t, resourcemodel.ResourceIdentity{}, deploymentOutput.Resources[0].Identity)
		require.NotEqual(t, resourcemodel.ResourceIdentity{}, deploymentOutput.Resources[1].Identity)
		require.Equal(t, testRendererOutput.SecretValues, deploymentOutput.SecretValues)
		require.Equal(t, map[string]any{renderers.DatabaseNameValue: "test-database", renderers.Host: testRendererOutput.ComputedValues[renderers.Host].Value}, deploymentOutput.ComputedValues)
	})

	t.Run("Verify deploy success with mongo recipe", func(t *testing.T) {
		resources := handlers.RecipeResponse{
			Resources: []string{cosmosAccountID, cosmosMongoID},
		}
		mocks.recipeHandler.EXPECT().DeployRecipe(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&resources, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(2).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

		testRendererOutput := buildRendererOutputMongo(modeRecipe)
		deploymentOutput, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.NoError(t, err)
		require.Equal(t, testRendererOutput.SecretValues, deploymentOutput.SecretValues)
		require.Equal(t, map[string]any{renderers.DatabaseNameValue: "test-database", "host": 8080}, deploymentOutput.ComputedValues)
		require.Equal(t, resources.Resources, deploymentOutput.RecipeData.Resources)
	})
	t.Run("Verify deploy success with redis recipe", func(t *testing.T) {
		resources := handlers.RecipeResponse{
			Resources: []string{"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Microsoft.Cache/Redis/testCache"},
			Secrets: map[string]any{
				"username":         "testUser",
				"password":         "testPassword",
				"connectionString": "test-connection-string",
			},
			Values: map[string]any{
				"host": "myrediscache.redis.cache.windows.net",
				"port": 6379,
			},
		}
		mocks.recipeHandler.EXPECT().DeployRecipe(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&resources, nil)

		testRendererOutput := buildRendererOutputRedis()
		deploymentOutput, err := dp.Deploy(ctx, redisLinkResourceId, testRendererOutput)
		require.NoError(t, err)
		require.Equal(t, testRendererOutput.SecretValues, deploymentOutput.SecretValues)
		require.Equal(t, map[string]any{renderers.Port: 6379, renderers.Host: "myrediscache.redis.cache.windows.net"}, deploymentOutput.ComputedValues)
		require.Equal(t, resources.Resources, deploymentOutput.RecipeData.Resources)
	})

	t.Run("Verify deploy failure with recipe", func(t *testing.T) {
		deploymentName := "recipe" + strconv.FormatInt(time.Now().UnixNano(), 10)
		mocks.recipeHandler.EXPECT().DeployRecipe(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&handlers.RecipeResponse{}, fmt.Errorf("failed to deploy recipe - %s", deploymentName))

		testRendererOutput := buildRendererOutputMongo(modeRecipe)
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "failed to deploy recipe - "+deploymentName, err.Error())
	})

	t.Run("Verify deploy failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, errors.New("failed to deploy the resource"))

		testRendererOutput := buildRendererOutputMongo(modeResource)
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "failed to deploy the resource", err.Error())
	})

	t.Run("Verify deploy failure - invalid request", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, v1.NewClientErrInvalidRequest("failed to access connected Azure resource"))

		testRendererOutput := buildRendererOutputMongo(modeResource)
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "failed to access connected Azure resource", err.(*v1.ErrClientRP).Message)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		testRendererOutput := buildRendererOutputMongo(modeResource)
		testRendererOutput.Resources[0].Dependencies = []rpv1.Dependency{
			{LocalID: ""},
		}
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "missing localID for outputresource", err.Error())
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		testRendererOutput := buildRendererOutputMongo(modeResource)
		testRendererOutput.Resources[0].ResourceType.Type = "foo"
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: azure, Type: foo' is unsupported", err.Error())
	})

	t.Run("Missing output resource identity", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

		testRendererOutput := buildRendererOutputMongo(modeResource)
		testRendererOutput.Resources[0].Identity = resourcemodel.ResourceIdentity{}
		testRendererOutput.Resources[1].Identity = resourcemodel.ResourceIdentity{}
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "output resource \"AzureCosmosAccount\" does not have an identity. This is a bug in the handler or renderer", err.Error())
	})

	t.Run("Recipe deployment - invalid resource id", func(t *testing.T) {
		resources := handlers.RecipeResponse{
			Resources: []string{"invalid-id"},
		}
		mocks.recipeHandler.EXPECT().DeployRecipe(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(&resources, nil)

		expectedErr := v1.NewClientErrInvalidRequest(fmt.Sprintf("failed to parse id \"%s\" of the resource deployed by recipe \"testRecipe\" for resource \"%s\": 'invalid-id' is not a valid resource id", resources.Resources[0], mongoLinkResourceID))
		testRendererOutput := buildRendererOutputMongo(modeRecipe)
		_, err := dp.Deploy(ctx, mongoLinkResourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, expectedErr, err)
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
	testOutputResource := rpv1.OutputResource{
		LocalID:      rpv1.LocalIDAzureCosmosAccount,
		ResourceType: testResourceType,
		Resource: map[string]any{
			"some-data": "jsonpointer-value",
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []rpv1.OutputResource{testOutputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-key1": {
				LocalID: rpv1.LocalIDAzureCosmosAccount,
				Value:   "static-value",
			},
			"test-key2": {
				LocalID:           rpv1.LocalIDAzureCosmosAccount,
				PropertyReference: "property-key",
			},
			"test-key3": {
				LocalID:     rpv1.LocalIDAzureCosmosAccount,
				JSONPointer: "/some-data",
			},
		},
	}

	expectedCosmosAccountIdentity := resourcemodel.ResourceIdentity{
		ResourceType: &testResourceType,
		Data:         resourcemodel.ARMIdentity{},
	}
	properties := map[string]string{"property-key": "property-value"}
	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(expectedCosmosAccountIdentity, properties, nil)

	deploymentOutput, err := dp.Deploy(ctx, mongoLinkResourceID, rendererOutput)
	require.NoError(t, err)

	expected := map[string]any{
		"test-key1": "static-value",
		"test-key2": "property-value",
		"test-key3": "jsonpointer-value",
	}
	require.Equal(t, expected, deploymentOutput.ComputedValues)
	require.Equal(t, expectedCosmosAccountIdentity, deploymentOutput.Resources[0].Identity)
}

func Test_Deploy_InvalidComputedValues(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	outputResource := rpv1.OutputResource{
		LocalID:      "test-local-id",
		ResourceType: resourceType,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourceType,
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []rpv1.OutputResource{outputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-value": {
				LocalID:     "test-local-id",
				JSONPointer: ".ddkfkdk",
			},
		},
	}

	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

	expectedErr := fmt.Sprintf("failed to parse JSON pointer \".ddkfkdk\" for computed value \"test-value\" for link \"%s\": JSON pointer must be empty or start with a \"/", mongoLinkResourceID)
	_, err := dp.Deploy(ctx, mongoLinkResourceID, rendererOutput)
	require.Error(t, err)
	require.Equal(t, expectedErr, err.Error())
}

func Test_Deploy_MissingJsonPointer(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	outputResource := rpv1.OutputResource{
		LocalID:      "test-local-id",
		ResourceType: resourceType,
		Identity: resourcemodel.ResourceIdentity{
			ResourceType: &resourceType,
			Data: resourcemodel.ARMIdentity{
				ID: "test",
			},
		},
		Resource: map[string]any{
			"some-data": 3,
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []rpv1.OutputResource{outputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-value": {
				LocalID:     "test-local-id",
				JSONPointer: "/some-other-data", // this key is missing
			},
		},
	}
	expectedErr := fmt.Sprintf("failed to process JSON pointer \"/some-other-data\" to fetch computed value \"test-value\". Output resource identity: %v. Link id: \"%s\": object has no key \"some-other-data\"",
		outputResource.Identity.Data, mongoLinkResourceID)

	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, nil)

	_, err := dp.Deploy(ctx, mongoLinkResourceID, rendererOutput)
	require.Error(t, err)
	require.Equal(t, expectedErr, err.Error())
}

func Test_Delete(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	testOutputResources := buildOutputResourcesMongo(modeRecipe)
	testResourceData := ResourceData{
		ID:              mongoLinkResourceID,
		OutputResources: testOutputResources,
	}

	t.Run("Verify deletion for mode resource", func(t *testing.T) {
		outputResources := buildOutputResourcesMongo(modeResource)
		resourceData := ResourceData{
			ID:              mongoLinkResourceID,
			OutputResources: outputResources,
		}
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)
		err := dp.Delete(ctx, resourceData)
		require.NoError(t, err)
	})

	t.Run("Verify delete success with recipe resources", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)

		err := dp.Delete(ctx, testResourceData)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		err := dp.Delete(ctx, testResourceData)
		require.Error(t, err)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{
			{
				LocalID: rpv1.LocalIDAzureCosmosDBMongo,
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
				Dependencies: []rpv1.Dependency{
					{
						LocalID: "",
					},
				},
			},
		}
		resourceData := ResourceData{
			OutputResources: outputResources,
			ID:              mongoLinkResourceID,
		}

		err := dp.Delete(ctx, resourceData)
		require.Error(t, err)
		require.Equal(t, "missing localID for outputresource", err.Error())
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		outputResources := []rpv1.OutputResource{
			{
				LocalID: rpv1.LocalIDAzureCosmosAccount,
				ResourceType: resourcemodel.ResourceType{
					Type:     "foo",
					Provider: resourcemodel.ProviderAzure,
				},
			},
		}
		resourceData := ResourceData{
			OutputResources: outputResources,
			ID:              mongoLinkResourceID,
		}
		err := dp.Delete(ctx, resourceData)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: azure, Type: foo' is unsupported", err.Error())
	})
}

func Test_Delete_Dapr(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	daprLinkResourceID := getResourceID(daprLinkID)
	testOutputResources := buildOutputResourcesDapr(modeResource)
	testResourceData := ResourceData{
		ID:              daprLinkResourceID,
		OutputResources: testOutputResources,
	}

	t.Run("Verify handler delete is invoked", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		err := dp.Delete(ctx, testResourceData)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		err := dp.Delete(ctx, testResourceData)
		require.Error(t, err)
	})
}

func Test_FetchSecretsWithValues(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	rendererOutput := buildRendererOutputMongo(modeValues)
	computedValues := map[string]any{
		renderers.DatabaseNameValue: mongoLinkName,
	}
	res := buildInputResourceMongo(modeValues)
	resourceData := ResourceData{
		ID:              mongoLinkResourceID,
		Resource:        &res,
		OutputResources: rendererOutput.Resources,
		SecretValues:    rendererOutput.SecretValues,
		ComputedValues:  computedValues,
	}
	secrets, err := dp.FetchSecrets(ctx, resourceData)
	require.NoError(t, err)
	require.Equal(t, 3, len(secrets))
	require.Equal(t, "testUser", secrets[renderers.UsernameStringValue])
	require.Equal(t, "testPassword", secrets[renderers.PasswordStringHolder])
	require.Equal(t, cosmosConnectionString, secrets[renderers.ConnectionStringValue])
}

func Test_FetchSecretsWithResource(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resource := buildInputResourceMongo(modeResource)
	rendererOutput := buildRendererOutputMongo(modeResource)
	computedValues := map[string]any{
		renderers.DatabaseNameValue: "test-database",
	}

	mocks.secretsValueClient.EXPECT().FetchSecret(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cosmosConnectionString, nil)

	resourceData := ResourceData{
		ID:              mongoLinkResourceID,
		Resource:        &resource,
		OutputResources: rendererOutput.Resources,
		SecretValues:    rendererOutput.SecretValues,
		ComputedValues:  computedValues,
	}

	expectedOutput := map[string]any{
		renderers.ConnectionStringValue: cosmosConnectionString + "/test-database",
	}
	secrets, err := dp.FetchSecrets(ctx, resourceData)
	require.NoError(t, err)
	require.Equal(t, 1, len(secrets))
	require.Equal(t, expectedOutput, secrets)
}

func Test_FetchSecretsWithRecipe(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resource := buildInputResourceMongo(modeRecipe)
	rendererOutput := buildRendererOutputMongo(modeRecipe)
	computedValues := map[string]any{
		renderers.DatabaseNameValue: "test-database",
	}

	mocks.secretsValueClient.EXPECT().FetchSecret(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(cosmosConnectionString, nil)

	resourceData := ResourceData{
		ID:              mongoLinkResourceID,
		Resource:        &resource,
		OutputResources: rendererOutput.Resources,
		SecretValues:    rendererOutput.SecretValues,
		ComputedValues:  computedValues,
	}

	expectedOutput := map[string]any{
		renderers.ConnectionStringValue: cosmosConnectionString + "/test-database",
	}
	secrets, err := dp.FetchSecrets(ctx, resourceData)
	require.NoError(t, err)
	require.Equal(t, 1, len(secrets))
	require.Equal(t, expectedOutput, secrets)
}

func Test_GetEnvironmentMetadata(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	recipeName := "cosmos-recipe"

	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}
	t.Run("successfully get recipe metadata", func(t *testing.T) {
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource(recipeName, &corerp_dm.Providers{Azure: corerp_dm.ProvidersAzure{Scope: "/subscriptions/testSub/resourceGroups/testGroup"}})
		env := er.Metadata.ID
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)

		envMetadata, err := dp.getEnvironmentMetadata(ctx, env, recipeName)
		require.NoError(t, err)
		require.Equal(t, "Applications.Link/MongoDatabases", envMetadata.RecipeLinkType)
		require.Equal(t, "br:sampleregistry.azureacr.io/radius/recipes/cosmosdb", envMetadata.RecipeTemplatePath)

	})

	t.Run("fail to get recipe metadata", func(t *testing.T) {
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		er := buildEnvironmentResource("cosmos-test", &corerp_dm.Providers{Azure: corerp_dm.ProvidersAzure{Scope: "/subscriptions/testSub/resourceGroups/testGroup"}})
		env := er.Metadata.ID
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(er, nil)

		_, err := dp.getEnvironmentMetadata(ctx, env, recipeName)
		require.Error(t, err)
		require.Equal(t, fmt.Sprintf("recipe with name %q does not exist in the environment %s", recipeName, env), err.Error())
	})
}
