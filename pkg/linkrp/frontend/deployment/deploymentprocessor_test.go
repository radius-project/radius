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

package deployment

import (
	"context"
	"errors"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/model"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
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
	cosmosConnectionString = "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255/test-database"

	daprLinkName          = "test-state-store"
	daprLinkID            = "/subscriptions/testSub/resourceGroups/testGroup/providers/Applications.Link/daprStateStore/test-state-store"
	daprLinkType          = "Applications.Link/daprStateStore"
	azureTableStorageID   = "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.Storage/storageAccounts/test-account/tableServices/default/tables/mytable"
	recipeName            = "testRecipe"
	stateStoreType        = "state.dapr"
	daprStateStoreVersion = "v1"

	resourceProvisioningRecipe = "recipe"
	resourceProvisioningManual = "manual"
)

var (
	mongoLinkResourceID = getResourceID(mongoLinkID)
	recipeParams        = map[string]any{
		"throughput": 400,
	}
)

func buildInputResourceMongo(resourceProvisioning string) (testResource datamodel.MongoDatabase) {
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

	if resourceProvisioning == resourceProvisioningRecipe {
		testResource.Properties.Recipe = linkrp.LinkRecipe{
			Name:       recipeName,
			Parameters: recipeParams,
		}
	}
	testResource.Properties.Secrets = datamodel.MongoDatabaseSecrets{
		Password:         "testPassword",
		ConnectionString: cosmosConnectionString,
	}

	return
}

func buildOutputResourcesMongo(resourceProvisioning string) []rpv1.OutputResource {
	radiusManaged := false
	if resourceProvisioning == resourceProvisioningRecipe {
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

func buildRendererOutputMongo(resourceProvisioning string) (rendererOutput renderers.RendererOutput) {
	computedValues := map[string]renderers.ComputedValueReference{}
	secretValues := map[string]rpv1.SecretValueReference{}
	if resourceProvisioning == resourceProvisioningRecipe {
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
			},
		}
	} else if resourceProvisioning != "recipe" {
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

	recipeData := linkrp.RecipeData{}
	if resourceProvisioning == resourceProvisioningRecipe {
		recipeData = linkrp.RecipeData{
			RecipeProperties: linkrp.RecipeProperties{
				LinkRecipe: linkrp.LinkRecipe{
					Name:       recipeName,
					Parameters: recipeParams,
				},
				TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
				EnvParameters: map[string]any{
					"name": "account-mongo-db",
				},
			},
			APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
		}
	}

	rendererOutput = renderers.RendererOutput{
		Resources:      buildOutputResourcesMongo(resourceProvisioning),
		SecretValues:   secretValues,
		ComputedValues: computedValues,
		RecipeData:     recipeData,
	}

	return
}

func buildOutputResourcesDapr(mode string) []rpv1.OutputResource {
	radiusManaged := false

	accountResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.DaprStateStoreAzureStorage,
		Provider: resourcemodel.ProviderAzure,
	}

	return []rpv1.OutputResource{
		{
			LocalID:      rpv1.LocalIDDaprStateStoreAzureStorage,
			ResourceType: accountResourceType,
			Resource: map[string]string{
				handlers.KubernetesNameKey:       daprLinkName,
				handlers.KubernetesNamespaceKey:  "radius-test",
				handlers.ApplicationName:         "testApplication",
				handlers.KubernetesAPIVersionKey: "dapr.io/v1alpha1",
				handlers.KubernetesKindKey:       "Component",
				handlers.ResourceName:            daprLinkName,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &accountResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         cosmosMongoID,
					APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				},
			},
			RadiusManaged: &radiusManaged,
		},
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
			{
				ResourceType: linkrp.DaprStateStoresResourceType,
				Renderer:     mockRenderer,
			},
		},
		[]model.OutputResourceModel{
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosAccount,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.DaprStateStoreAzureStorage,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureRedis,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				// Handles all AWS types
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AnyResourceType,
					Provider: resourcemodel.ProviderAWS,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				// Handles all Kubernetes types
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AnyResourceType,
					Provider: resourcemodel.ProviderKubernetes,
				},
				ResourceHandler: mockResourceHandler,
			},
		},
		map[string]bool{
			resourcemodel.ProviderKubernetes: true,
			resourcemodel.ProviderAzure:      true,
			resourcemodel.ProviderAWS:        true,
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

func Test_Delete(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	testOutputResources := buildOutputResourcesMongo(resourceProvisioningRecipe)

	t.Run("Verify deletion for resourceProvisioning manual", func(t *testing.T) {
		outputResources := buildOutputResourcesMongo(resourceProvisioningManual)
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)
		err := dp.Delete(ctx, mongoLinkResourceID, outputResources)
		require.NoError(t, err)
	})

	t.Run("Verify delete success with recipe resources", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)

		err := dp.Delete(ctx, mongoLinkResourceID, testOutputResources)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		err := dp.Delete(ctx, mongoLinkResourceID, testOutputResources)
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

		err := dp.Delete(ctx, mongoLinkResourceID, outputResources)
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
		err := dp.Delete(ctx, mongoLinkResourceID, outputResources)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: azure, Type: foo' is unsupported", err.Error())
	})
}

func Test_Delete_Dapr(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	daprLinkResourceID := getResourceID(daprLinkID)
	testOutputResources := buildOutputResourcesDapr(resourceProvisioningManual)

	t.Run("Verify handler delete is invoked", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		err := dp.Delete(ctx, daprLinkResourceID, testOutputResources)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		err := dp.Delete(ctx, daprLinkResourceID, testOutputResources)
		require.Error(t, err)
	})
}

func Test_FetchSecretsWithValues(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	rendererOutput := buildRendererOutputMongo(resourceProvisioningManual)
	computedValues := map[string]any{
		renderers.DatabaseNameValue: mongoLinkName,
	}
	res := buildInputResourceMongo(resourceProvisioningManual)
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

func Test_FetchSecretsWithRecipe(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	resource := buildInputResourceMongo(resourceProvisioningRecipe)
	rendererOutput := buildRendererOutputMongo(resourceProvisioningRecipe)
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
		renderers.ConnectionStringValue: cosmosConnectionString,
	}
	secrets, err := dp.FetchSecrets(ctx, resourceData)
	require.NoError(t, err)
	require.Equal(t, 1, len(secrets))
	require.Equal(t, expectedOutput, secrets)
}
