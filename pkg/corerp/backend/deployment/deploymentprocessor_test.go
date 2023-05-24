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
	"encoding/json"
	"errors"
	"os"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/linkrp"
	linkrp_dm "github.com/project-radius/radius/pkg/linkrp/datamodel"
	linkrp_renderers "github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/linkrp/renderers/mongodatabases"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/test/testutil"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type SharedMocks struct {
	model              model.ApplicationModel
	db                 *store.MockStorageClient
	dbProvider         *dataprovider.MockDataStorageProvider
	resourceHandler    *handlers.MockResourceHandler
	renderer           *renderers.MockRenderer
	secretsValueClient *sv.MockSecretValueClient
	mctrl              *gomock.Controller
}

func setup(t *testing.T) SharedMocks {
	ctrl := gomock.NewController(t)

	renderer := renderers.NewMockRenderer(ctrl)
	resourceHandler := handlers.NewMockResourceHandler(ctrl)

	model := model.NewModel(
		[]model.RadiusResourceModel{
			{
				ResourceType: container.ResourceType,
				Renderer:     renderer,
			},
		},
		[]model.OutputResourceModel{
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.Deployment,
					Provider: resourcemodel.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.Secret,
					Provider: resourcemodel.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.Service,
					Provider: resourcemodel.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureCosmosDBMongo,
					Provider: resourcemodel.ProviderAzure,
				},
				ResourceHandler:        resourceHandler,
				SecretValueTransformer: &mongodatabases.AzureTransformer{},
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
		resourceHandler:    resourceHandler,
		renderer:           renderer,
		secretsValueClient: sv.NewMockSecretValueClient(ctrl),
		mctrl:              ctrl,
	}
}

func getTestResource() datamodel.ContainerResource {
	rawDataModel := testutil.ReadFixture("containerresourcedatamodel.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getLowerCaseTestResource() datamodel.ContainerResource {
	rawDataModel := testutil.ReadFixture("containerresourcedatamodellowercase.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getUpperCaseTestResource() datamodel.ContainerResource {
	rawDataModel := testutil.ReadFixture("containerresourcedatamodeluppercase.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getTestRendererOutput() renderers.RendererOutput {
	testOutputResources := []rpv1.OutputResource{
		{
			LocalID: rpv1.LocalIDService,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: resourcemodel.ProviderKubernetes,
			},
		},
	}

	rendererOutput := renderers.RendererOutput{
		Resources: testOutputResources,
		ComputedValues: map[string]rpv1.ComputedValueReference{
			"url": {
				Value: "http://test-application/test-route:8080",
			},
		},
	}
	return rendererOutput
}

func getTestResourceID(id string) resources.ID {
	resourceID, err := resources.ParseResource(id)
	if err != nil {
		panic(err)
	}

	return resourceID
}

func buildMongoDBLinkWithRecipe() linkrp_dm.MongoDatabase {
	return linkrp_dm.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Link/mongoDatabases/test-mongo",
			},
		},
		Properties: linkrp_dm.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
			Mode: linkrp_dm.LinkModeRecipe,
		},
		LinkMetadata: linkrp_dm.LinkMetadata{
			RecipeData: linkrp.RecipeData{
				RecipeProperties: linkrp.RecipeProperties{
					LinkRecipe: linkrp.LinkRecipe{
						Name: "mongoDB",
						Parameters: map[string]any{
							"ResourceGroup": "testRG",
							"Subscription":  "Radius-Test",
						},
					},
					TemplatePath: "testpublicrecipe.azurecr.io/bicep/modules/mongodatabases:v1",
				},
				APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				Resources: []string{"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
					"/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database"},
			},
		},
	}
}

func buildMongoDBResourceDataWithRecipeAndSecrets() ResourceData {
	testResource := buildMongoDBLinkWithRecipe()

	secretValues := map[string]rpv1.SecretValueReference{}
	secretValues[linkrp_renderers.ConnectionStringValue] = rpv1.SecretValueReference{
		LocalID:       rpv1.LocalIDAzureCosmosAccount,
		Action:        "listConnectionStrings",
		ValueSelector: "/connectionStrings/0/connectionString",
		Transformer: resourcemodel.ResourceType{
			Provider: resourcemodel.ProviderAzure,
			Type:     resourcekinds.AzureCosmosDBMongo,
		},
	}

	computedValues := map[string]any{
		linkrp_renderers.DatabaseNameValue: "db",
	}

	testResource.ComputedValues = computedValues
	testResource.SecretValues = secretValues

	accountResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosAccount,
		Provider: resourcemodel.ProviderAzure,
	}
	dbResourceType := resourcemodel.ResourceType{
		Type:     resourcekinds.AzureCosmosDBMongo,
		Provider: resourcemodel.ProviderAzure,
	}
	outputResources := []rpv1.OutputResource{
		{
			LocalID:              rpv1.LocalIDAzureCosmosAccount,
			ResourceType:         accountResourceType,
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts,
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &accountResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account",
					APIVersion: clientv2.DocumentDBManagementClientAPIVersion,
				},
			},
			RadiusManaged: to.Ptr(true),
		},
		{
			LocalID:              rpv1.LocalIDAzureCosmosDBMongo,
			ResourceType:         dbResourceType,
			ProviderResourceType: azresources.DocumentDBDatabaseAccounts + "/" + azresources.DocumentDBDatabaseAccountsMongoDBDatabases,
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &dbResourceType,
				Data: resourcemodel.ARMIdentity{
					ID:         "/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database",
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
			RadiusManaged: to.Ptr(true),
			Dependencies:  []rpv1.Dependency{{LocalID: rpv1.LocalIDAzureCosmosAccount}},
		},
	}

	return ResourceData{
		ID:              getTestResourceID(testResource.ID),
		OutputResources: outputResources,
		Resource:        &testResource,
		ComputedValues:  computedValues,
		SecretValues:    secretValues,
		RecipeData:      testResource.RecipeData}
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

	env := datamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				Kind: rpv1.KubernetesComputeKind,
				KubernetesCompute: rpv1.KubernetesComputeProperties{
					Namespace: "radius-test",
				},
			},
		},
	}

	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil, nil}

	t.Run("verify render success", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		depId2, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Link/mongoDatabases/test-mongo")
		requiredResources := []resources.ID{depId1, depId2}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(5).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)
		application := datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
		}
		ar := store.Object{
			Metadata: store.Metadata{
				ID: application.ID,
			},
			Data: application,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)
		er := store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}

		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		mongoResource := linkrp_dm.MongoDatabase{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Link/mongoDatabases/test-mongo",
				},
			},
			Properties: linkrp_dm.MongoDatabaseProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
				Mode: linkrp_dm.LinkModeValues,
			},
		}
		mr := store.Object{
			Metadata: store.Metadata{
				ID: mongoResource.ID,
			},
			Data: mongoResource,
		}

		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&mr, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render success lowercase resourcetype", func(t *testing.T) {
		testResource := getLowerCaseTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(4).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)
		application := datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
		}
		ar := store.Object{
			Metadata: store.Metadata{
				ID: application.ID,
			},
			Data: application,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)
		er := store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render success uppercase resourcetype", func(t *testing.T) {
		testResource := getUpperCaseTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(4).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)
		application := datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
		}
		ar := store.Object{
			Metadata: store.Metadata{
				ID: application.ID,
			},
			Data: application,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)
		er := store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, &testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render error", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(4).Return(mocks.db, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render the resource"))

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)
		application := datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
		}
		ar := store.Object{
			Metadata: store.Metadata{
				ID: application.ID,
			},
			Data: application,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)
		er := store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "failed to render the resource")
	})

	t.Run("Failure to get storage client", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("unsupported storage provider"))

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "failed to fetch the resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\". Err: unsupported storage provider", err.Error())
	})

	t.Run("Resource not found in data store", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&store.Object{}, &store.ErrNotFound{})

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\" does not exist", err.(*v1.ErrClientRP).Message)
	})

	t.Run("Data store access error", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&store.Object{}, errors.New("failed to connect to data store"))

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "failed to fetch the resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\". Err: failed to connect to data store", err.Error())
	})

	t.Run("Invalid resource type", func(t *testing.T) {
		testInvalidResourceID := "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.foo/foo/foo"
		testResource := getTestResource()
		resourceID := getTestResourceID(testInvalidResourceID)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "radius resource type 'Applications.foo/foo' is unsupported")
	})

	t.Run("Invalid application id", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		testResource.Properties.Application = "invalid-app-id"

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "application ID \"invalid-app-id\" for the resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\" is not a valid id. Error: 'invalid-app-id' is not a valid resource id", err.(*v1.ErrClientRP).Message)
	})

	t.Run("Missing application id", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		testResource.Properties.Application = ""

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, "missing required application id for the resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\"", err.Error())
	})

	t.Run("Invalid application resource type", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		testResource.Properties.Application = "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/app/test-application"

		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err)
		require.Equal(t, v1.CodeInvalid, err.(*v1.ErrClientRP).Code)
		require.Equal(t, "linked \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/app/test-application\" has invalid Applications.Core/applications resource type.", err.(*v1.ErrClientRP).Message)
	})

	t.Run("Missing output resource provider", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Provider = ""
		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(4).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)
		application := datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
		}
		ar := store.Object{
			Metadata: store.Metadata{
				ID: application.ID,
			},
			Data: application,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)
		er := store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "output resource \"Deployment\" does not have a provider specified")
	})

	t.Run("Unsupported output resource provider", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Provider = "unknown"
		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(4).Return(mocks.db, nil)

		cr := store.Object{
			Metadata: store.Metadata{
				ID: testResource.ID,
			},
			Data: testResource,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)
		application := datamodel.Application{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
			Properties: datamodel.ApplicationProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
		}
		ar := store.Object{
			Metadata: store.Metadata{
				ID: application.ID,
			},
			Data: application,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)
		er := store.Object{
			Metadata: store.Metadata{
				ID: env.ID,
			},
			Data: env,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Application: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
				},
			},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)

		_, err := dp.Render(ctx, resourceID, &testResource)
		require.Error(t, err, "provider unknown is not configured. Cannot support resource type azure.roleassignment")
	})
}

func Test_Deploy(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil, nil}

	t.Run("Verify deploy success", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)
		kubeProp := map[string]string{
			handlers.KubernetesKindKey:       resourcekinds.Service,
			handlers.KubernetesAPIVersionKey: "v1",
			handlers.KubernetesNamespaceKey:  "test-namespace",
			handlers.ResourceName:            "test-deployment",
		}

		expectedIdentity := resourcemodel.ResourceIdentity{
			ResourceType: &resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: resourcemodel.ProviderKubernetes,
			},
			Data: resourcemodel.KubernetesIdentity{
				Name:       kubeProp[handlers.ResourceName],
				Namespace:  kubeProp[handlers.KubernetesNamespaceKey],
				Kind:       kubeProp[handlers.KubernetesKindKey],
				APIVersion: kubeProp[handlers.ResourceName],
			},
		}

		mocks.resourceHandler.
			EXPECT().
			Put(gomock.Any(), gomock.Any()).Times(1).
			DoAndReturn(func(ctx context.Context, options *handlers.PutOptions) (map[string]string, error) {
				options.Resource.Identity = expectedIdentity
				return kubeProp, nil
			})

		deploymentOutput, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(deploymentOutput.DeployedOutputResources))
		require.NotEqual(t, resourcemodel.ResourceIdentity{}, deploymentOutput.DeployedOutputResources[0].Identity)
		require.Equal(t, map[string]any{"url": testRendererOutput.ComputedValues["url"].Value}, deploymentOutput.ComputedValues)
	})

	t.Run("Verify deploy failure", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("failed to deploy the resource"))

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].Dependencies = []rpv1.Dependency{
			{LocalID: ""},
		}

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.ErrorContains(t, err, "missing localID for outputresource")
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Type = "foo"

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.ErrorContains(t, err, "output resource kind 'Provider: kubernetes, Type: foo' is unsupported")
	})

	t.Run("Missing output resource identity", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.
			EXPECT().
			Put(gomock.Any(), gomock.Any()).Times(1).
			DoAndReturn(func(ctx context.Context, options *handlers.PutOptions) (map[string]string, error) {
				options.Resource.Identity = resourcemodel.ResourceIdentity{}
				return map[string]string{}, nil
			})

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.ErrorContains(t, err, `output resource "Service" does not have an identity. This is a bug in the handler`)
	})
}

func Test_Delete(t *testing.T) {
	ctx := createContext(t)

	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil, nil}

	t.Run("Verify delete success", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.Error(t, err)
	})

	t.Run("Verify delete with no output resources", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		testResource.Properties.Status.OutputResources = []rpv1.OutputResource{}

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0).Return(nil)

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.NoError(t, err)
	})
}

func Test_getEnvOptions_PublicEndpointOverride(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, nil, nil, nil, nil}

	env := &datamodel.Environment{
		Properties: datamodel.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				Kind: rpv1.KubernetesComputeKind,
				KubernetesCompute: rpv1.KubernetesComputeProperties{
					Namespace: "radius-system",
				},
				Identity: &rpv1.IdentitySettings{},
			},
			Providers: datamodel.Providers{
				Azure: datamodel.ProvidersAzure{
					Scope: "/subscriptions/subid/resourceGroups/rgName",
				},
			},
		},
	}

	t.Run("Verify getEnvOptions succeeds (host:port)", func(t *testing.T) {
		os.Setenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE", "localhost:8000")
		defer os.Unsetenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

		options, err := dp.getEnvOptions(ctx, env)
		require.NoError(t, err)

		require.True(t, options.Gateway.PublicEndpointOverride)
		require.Equal(t, options.Gateway.Hostname, "localhost")
		require.Equal(t, options.Gateway.Port, "8000")
		require.Equal(t, options.Gateway.ExternalIP, "")
	})

	t.Run("Verify getEnvOptions succeeds (host)", func(t *testing.T) {
		os.Setenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE", "www.contoso.com")
		defer os.Unsetenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

		options, err := dp.getEnvOptions(ctx, env)
		require.NoError(t, err)

		require.True(t, options.Gateway.PublicEndpointOverride)
		require.Equal(t, options.Gateway.Hostname, "www.contoso.com")
		require.Equal(t, options.Gateway.Port, "")
		require.Equal(t, options.Gateway.ExternalIP, "")
	})

	t.Run("Verify getEnvOptions fails (URL)", func(t *testing.T) {
		os.Setenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE", "http://localhost:8000")
		defer os.Unsetenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

		options, err := dp.getEnvOptions(ctx, env)
		require.Error(t, err)
		require.EqualError(t, err, "a URL is not accepted here. Please reinstall Radius with a valid public endpoint using rad install kubernetes --reinstall --public-endpoint-override <your-endpoint>")
		require.Equal(t, options, renderers.EnvironmentOptions{})
	})
}

func Test_getResourceDataByID(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil, nil}

	t.Run("Get recipe data from connected mongoDB resources", func(t *testing.T) {
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		depId, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Link/mongoDatabases/test-mongo")
		mongoResource := buildMongoDBLinkWithRecipe()
		mr := store.Object{
			Metadata: store.Metadata{
				ID: mongoResource.ID,
			},
			Data: mongoResource,
		}

		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&mr, nil)

		resourceData, err := dp.getResourceDataByID(ctx, depId)
		require.NoError(t, err)
		require.Equal(t, resourceData.RecipeData, mongoResource.RecipeData)
	})
}

func Test_fetchSecrets(t *testing.T) {
	ctx := createContext(t)

	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, nil, mocks.secretsValueClient, nil, nil}

	t.Run("Get secrets from recipe data when resource has associated recipe", func(t *testing.T) {
		mongoResource := buildMongoDBResourceDataWithRecipeAndSecrets()
		secret := "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255/db?ssl=true"
		mocks.secretsValueClient.EXPECT().FetchSecret(ctx, gomock.Any(), mongoResource.SecretValues[linkrp_renderers.ConnectionStringValue].Action, mongoResource.SecretValues[linkrp_renderers.ConnectionStringValue].ValueSelector).Times(1).Return(secret, nil)
		secretValues, err := dp.FetchSecrets(ctx, mongoResource)
		require.NoError(t, err)
		require.Equal(t, 1, len(secretValues))
		require.Equal(t, secret, secretValues[linkrp_renderers.ConnectionStringValue])
	})
}
