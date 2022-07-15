// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/handlers"
	"github.com/project-radius/radius/pkg/connectorrp/model"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/connectorrp/renderers/mongodatabases"
	corerpDatamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
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
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Resource:    "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	azureMongoOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: providers.ProviderAzure,
			},
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: providers.ProviderAzure,
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
					Provider: providers.ProviderAzure,
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
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
				Resource:    "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
			},
		},
	}

	azureMongoOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDAzureCosmosAccount,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: providers.ProviderAzure,
			},
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: providers.ProviderAzure,
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
					Provider: providers.ProviderAzure,
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

func buildFetchSecretsInput() ResourceData {
	resourceID, testResource, rendererOutput := buildTestMongoResource()
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

	return ResourceData{resourceID, testResource, rendererOutput.Resources, computedValues, secretValues}
}

type SharedMocks struct {
	model              model.ApplicationModel
	db                 *store.MockStorageClient
	dbProvider         *dataprovider.MockDataStorageProvider
	resourceHandler    *handlers.MockResourceHandler
	renderer           *renderers.MockRenderer
	secretsValueClient *renderers.MockSecretValueClient
	storageProvider    *dataprovider.MockDataStorageProvider
}

func setup(t *testing.T) SharedMocks {
	ctrl := gomock.NewController(t)

	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)

	model := model.NewModel(
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
					Provider: providers.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: providers.ProviderAzure,
				},
				ResourceHandler: mockResourceHandler,
			},
		},
		map[string]bool{
			providers.ProviderKubernetes: true,
			providers.ProviderAzure:      true,
		})

	return SharedMocks{
		model:              model,
		db:                 store.NewMockStorageClient(ctrl),
		dbProvider:         dataprovider.NewMockDataStorageProvider(ctrl),
		resourceHandler:    mockResourceHandler,
		renderer:           mockRenderer,
		secretsValueClient: renderers.NewMockSecretValueClient(ctrl),
	}
}

func getResourceID(id string) resources.ID {
	resourceID, err := resources.Parse(id)
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

func Test_Render_Success(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}
	t.Run("verify render success", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResource()

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		environment := corerpDatamodel.Environment{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
			Properties: corerpDatamodel.EnvironmentProperties{
				Compute: corerpDatamodel.EnvironmentCompute{
					KubernetesCompute: corerpDatamodel.KubernetesComputeProperties{
						Namespace: "radius-test",
					},
				},
			},
		}
		er := store.Object{
			Metadata: store.Metadata{
				ID: environment.ID,
			},
			Data: environment,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render success with mixedcase resourcetype", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResourceMixedCaseResourceType()

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		environment := corerpDatamodel.Environment{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
			Properties: corerpDatamodel.EnvironmentProperties{
				Compute: corerpDatamodel.EnvironmentCompute{
					KubernetesCompute: corerpDatamodel.KubernetesComputeProperties{
						Namespace: "radius-test",
					},
				},
			},
		}
		er := store.Object{
			Metadata: store.Metadata{
				ID: environment.ID,
			},
			Data: environment,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render error", func(t *testing.T) {
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render the resource"))
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		environment := corerpDatamodel.Environment{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
			Properties: corerpDatamodel.EnvironmentProperties{
				Compute: corerpDatamodel.EnvironmentCompute{
					KubernetesCompute: corerpDatamodel.KubernetesComputeProperties{
						Namespace: "radius-test",
					},
				},
			},
		}
		er := store.Object{
			Metadata: store.Metadata{
				ID: environment.ID,
			},
			Data: environment,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		resourceID, testResource, _ := buildTestMongoResource()
		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "failed to render the resource")
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
					Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
					Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/env0",
					Resource:    "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
				},
			},
		}

		_, err := dp.Render(ctx, parsedID, &testInvalidResource)
		require.Error(t, err, "radius resource type 'Applications.foo/foo' is unsupported")

	})

	t.Run("Missing output resource provider", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResource()
		testRendererOutput.Resources[0].ResourceType.Provider = ""

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		environment := corerpDatamodel.Environment{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
			Properties: corerpDatamodel.EnvironmentProperties{
				Compute: corerpDatamodel.EnvironmentCompute{
					KubernetesCompute: corerpDatamodel.KubernetesComputeProperties{
						Namespace: "radius-test",
					},
				},
			},
		}
		er := store.Object{
			Metadata: store.Metadata{
				ID: environment.ID,
			},
			Data: environment,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "output resource \"AzureCosmosAccount\" does not have a provider specified")
	})

	t.Run("Unsupported output resource provider", func(t *testing.T) {
		resourceID, testResource, testRendererOutput := buildTestMongoResource()
		testRendererOutput.Resources[0].ResourceType.Provider = "unknown"

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		environment := corerpDatamodel.Environment{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
			Properties: corerpDatamodel.EnvironmentProperties{
				Compute: corerpDatamodel.EnvironmentCompute{
					KubernetesCompute: corerpDatamodel.KubernetesComputeProperties{
						Namespace: "radius-test",
					},
				},
			},
		}
		er := store.Object{
			Metadata: store.Metadata{
				ID: environment.ID,
			},
			Data: environment,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "provider unknown is not configured. Cannot support resource type azure.cosmosdb.account")
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
				Provider: providers.ProviderAzure,
			},
			Data: resourcemodel.ARMIdentity{},
		}

		expectedCosmosMongoAccountIdentity := resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosAccount,
				Provider: providers.ProviderAzure,
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

	t.Run("Verify deploy failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, map[string]string{}, errors.New("failed to deploy the resource"))

		resourceID, _, testRendererOutput := buildTestMongoResource()
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
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
		Provider: providers.ProviderAzure,
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
			Provider: providers.ProviderAzure,
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
		Provider: providers.ProviderAzure,
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
		Provider: providers.ProviderAzure,
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
				Provider: providers.ProviderAzure,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosAccount,
					Provider: providers.ProviderAzure,
				},
				Data: resourcemodel.ARMIdentity{},
			},
		},
		{
			LocalID: outputresource.LocalIDAzureCosmosDBMongo,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureCosmosDBMongo,
				Provider: providers.ProviderAzure,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: providers.ProviderAzure,
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
		err := dp.Delete(ctx, resourceID, testOutputResources)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		resourceID, _, _ := buildTestMongoResource()
		err := dp.Delete(ctx, resourceID, testOutputResources)
		require.Error(t, err)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		resourceID, _, _ := buildTestMongoResource()
		outputResources := []outputresource.OutputResource{
			{
				LocalID: outputresource.LocalIDAzureCosmosDBMongo,
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: providers.ProviderAzure,
				},
				Identity: resourcemodel.ResourceIdentity{
					ResourceType: &resourcemodel.ResourceType{
						Type:     resourcekinds.AzureCosmosDBMongo,
						Provider: providers.ProviderAzure,
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
		err := dp.Delete(ctx, resourceID, outputResources)
		require.Error(t, err)
		require.Equal(t, "missing localID for outputresource", err.Error())
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		outputResources := []outputresource.OutputResource{
			{
				LocalID: outputresource.LocalIDAzureCosmosAccount,
				ResourceType: resourcemodel.ResourceType{
					Type:     "foo",
					Provider: providers.ProviderAzure,
				},
			},
		}
		resourceID, _, _ := buildTestMongoResource()
		err := dp.Delete(ctx, resourceID, outputResources)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: azure, Type: foo' is unsupported", err.Error())
	})
}

func Test_FetchSecrets(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.storageProvider, mocks.secretsValueClient, nil}

	input := buildFetchSecretsInput()
	secrets, err := dp.FetchSecrets(ctx, input)
	require.NoError(t, err)
	require.Equal(t, 3, len(secrets))
}
