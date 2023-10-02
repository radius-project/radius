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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/azure/clientv2"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/corerp/handlers"
	"github.com/radius-project/radius/pkg/corerp/model"
	"github.com/radius-project/radius/pkg/corerp/renderers"
	"github.com/radius-project/radius/pkg/corerp/renderers/container"
	dsrp_dm "github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	pr_dm "github.com/radius-project/radius/pkg/portableresources/datamodel"
	pr_renderers "github.com/radius-project/radius/pkg/portableresources/renderers"
	"github.com/radius-project/radius/pkg/resourcemodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/testutil"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

type SharedMocks struct {
	model           model.ApplicationModel
	db              *store.MockStorageClient
	dbProvider      *dataprovider.MockDataStorageProvider
	resourceHandler *handlers.MockResourceHandler
	renderer        *renderers.MockRenderer
	mctrl           *gomock.Controller
	testApp         datamodel.Application
	testEnv         datamodel.Environment
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
					Type:     model.AnyResourceType,
					Provider: resourcemodel.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_azure.ResourceTypeAuthorizationRoleAssignment,
					Provider: resourcemodel.ProviderAzure,
				},
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     "Test.Namespace/testResources",
					Provider: "test",
				},
				ResourceHandler: resourceHandler,
			},
		},
		map[string]bool{
			resourcemodel.ProviderKubernetes: true,
			resourcemodel.ProviderAzure:      true,
		})

	app := datamodel.Application{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
			},
		},
		Properties: datamodel.ApplicationProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
			},
		},
	}

	env := datamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
				Name: "test-env",
			},
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				Kind: "kubernetes",
				KubernetesCompute: rpv1.KubernetesComputeProperties{
					ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
					Namespace:  "default",
				},
			},
		},
	}

	return SharedMocks{
		model:           model,
		db:              store.NewMockStorageClient(ctrl),
		dbProvider:      dataprovider.NewMockDataStorageProvider(ctrl),
		resourceHandler: resourceHandler,
		renderer:        renderer,
		mctrl:           ctrl,
		testApp:         app,
		testEnv:         env,
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
			CreateResource: &rpv1.Resource{
				ResourceType: resourcemodel.ResourceType{
					Type:     resources_kubernetes.ResourceTypeService,
					Provider: resourcemodel.ProviderKubernetes,
				},
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

func buildMongoDBWithRecipe() dsrp_dm.MongoDatabase {
	return dsrp_dm.MongoDatabase{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Datastores/mongoDatabases/test-mongo",
			},
		},
		Properties: dsrp_dm.MongoDatabaseProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Application: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/applications/testApplication",
				Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
			},
		},
		PortableResourceMetadata: pr_dm.PortableResourceMetadata{
			RecipeData: portableresources.RecipeData{
				RecipeProperties: portableresources.RecipeProperties{
					ResourceRecipe: portableresources.ResourceRecipe{
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
	testResource := buildMongoDBWithRecipe()

	secretValues := map[string]rpv1.SecretValueReference{}
	secretValues[pr_renderers.ConnectionStringValue] = rpv1.SecretValueReference{
		Value: "test-connection-string",
	}

	computedValues := map[string]any{
		pr_renderers.DatabaseNameValue: "db",
	}

	testResource.ComputedValues = computedValues
	testResource.SecretValues = secretValues

	dbResourceType := resourcemodel.ResourceType{
		Type:     "Microsoft.DocumentDB/databaseAccounts/mongodbDatabases",
		Provider: resourcemodel.ProviderAzure,
	}
	outputResources := []rpv1.OutputResource{
		{
			LocalID:       rpv1.LocalIDAzureCosmosAccount,
			ID:            resources.MustParse("/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account"),
			RadiusManaged: to.Ptr(true),
		},
		{
			LocalID: rpv1.LocalIDAzureCosmosDBMongo,
			ID:      resources.MustParse("/subscriptions/test-sub/resourceGroups/test-group/providers/Microsoft.DocumentDB/databaseAccounts/test-account/mongodbDatabases/test-database"),
			CreateResource: &rpv1.Resource{
				ResourceType: dbResourceType,
				Data: map[string]any{
					"properties": map[string]any{
						"resource": map[string]string{
							"id": "test-database",
						},
					},
				},
				Dependencies: []string{rpv1.LocalIDAzureCosmosAccount},
			},
			RadiusManaged: to.Ptr(true),
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

func Test_Render(t *testing.T) {
	ctx := testcontext.New(t)

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
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

	t.Run("verify render success", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		depId1, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		depId2, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Datastores/mongoDatabases/test-mongo")
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

		mongoResource := dsrp_dm.MongoDatabase{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Datastores/mongoDatabases/test-mongo",
				},
			},
			Properties: dsrp_dm.MongoDatabaseProperties{
				BasicResourceProperties: rpv1.BasicResourceProperties{
					Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
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

		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&store.Object{}, &store.ErrNotFound{ID: testResource.ID})

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

		testRendererOutput.Resources[0].CreateResource.ResourceType.Provider = ""
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

		testRendererOutput.Resources[0].CreateResource.ResourceType.Provider = "unknown"
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

func setupDeployMocks(mocks SharedMocks, simulated bool) {
	testResource := getTestResource()
	mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).AnyTimes().Return(mocks.db, nil)
	cr := store.Object{
		Metadata: store.Metadata{
			ID: testResource.ID,
		},
		Data: testResource,
	}
	mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&cr, nil)

	app := datamodel.Application{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/applications/test-application",
			},
		},
		Properties: datamodel.ApplicationProperties{
			BasicResourceProperties: rpv1.BasicResourceProperties{
				Environment: "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
			},
		},
	}

	ar := store.Object{
		Metadata: store.Metadata{
			ID: mocks.testApp.ID,
		},
		Data: app,
	}
	mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&ar, nil)

	env := datamodel.Environment{
		BaseResource: v1.BaseResource{
			TrackedResource: v1.TrackedResource{
				ID:   "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.Core/environments/test-env",
				Name: "test-env",
			},
		},
		Properties: datamodel.EnvironmentProperties{
			Compute: rpv1.EnvironmentCompute{
				Kind: "kubernetes",
				KubernetesCompute: rpv1.KubernetesComputeProperties{
					ResourceID: "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster",
					Namespace:  "default",
				},
			},
		},
	}

	if simulated {
		env.Properties.Simulated = true
	}

	er := store.Object{
		Metadata: store.Metadata{
			ID: mocks.testEnv.ID,
		},
		Data: env,
	}
	mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&er, nil)
}

func Test_Deploy(t *testing.T) {
	t.Run("Verify deploy success", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)
		kubeProp := map[string]string{
			handlers.KubernetesKindKey:       "Service",
			handlers.KubernetesAPIVersionKey: "v1",
			handlers.KubernetesNamespaceKey:  "test-namespace",
			handlers.ResourceName:            "test-deployment",
		}

		expectedID := resources_kubernetes.IDFromParts(
			resources_kubernetes.PlaneNameTODO,
			"",
			kubeProp[handlers.KubernetesKindKey],
			kubeProp[handlers.KubernetesNamespaceKey],
			kubeProp[handlers.ResourceName])

		setupDeployMocks(mocks, false)

		mocks.resourceHandler.
			EXPECT().
			Put(gomock.Any(), gomock.Any()).Times(1).
			DoAndReturn(func(ctx context.Context, options *handlers.PutOptions) (map[string]string, error) {
				options.Resource.ID = expectedID
				return kubeProp, nil
			})

		deploymentOutput, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(deploymentOutput.DeployedOutputResources))
		require.NotEqual(t, resources.ID{}, deploymentOutput.DeployedOutputResources[0].ID)
		require.Equal(t, map[string]any{"url": testRendererOutput.ComputedValues["url"].Value}, deploymentOutput.ComputedValues)
	})

	t.Run("Verify deploy success with simulated env", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		setupDeployMocks(mocks, true)

		// Note: No PUT call is made on the mocks to actually deploy the resource
		deploymentOutput, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(deploymentOutput.DeployedOutputResources))
	})

	t.Run("Verify deploy failure", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		setupDeployMocks(mocks, false)

		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("failed to deploy the resource"))

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].CreateResource.Dependencies = []string{""}

		setupDeployMocks(mocks, false)

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.ErrorContains(t, err, "missing localID for outputresource")
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].CreateResource.ResourceType = resourcemodel.ResourceType{Provider: resourcemodel.ProviderAzure, Type: "foo"}

		setupDeployMocks(mocks, false)

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.ErrorContains(t, err, "output resource kind 'Provider: azure, Type: foo' is unsupported")
	})

	t.Run("Missing output resource identity", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.
			EXPECT().
			Put(gomock.Any(), gomock.Any()).Times(1).
			DoAndReturn(func(ctx context.Context, options *handlers.PutOptions) (map[string]string, error) {
				options.Resource.ID = resources.ID{}
				return map[string]string{}, nil
			})

		setupDeployMocks(mocks, false)
		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)

		require.ErrorContains(t, err, `output resource "Service" does not have an id. This is a bug in the handler`)
	})
}

func Test_Delete(t *testing.T) {

	t.Run("Verify delete success", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.NoError(t, err)
	})

	t.Run("Verify delete failure", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete the resource"))

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.Error(t, err)
	})

	t.Run("Verify delete with no output resources", func(t *testing.T) {
		ctx := testcontext.New(t)
		mocks := setup(t)
		dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		testResource.Properties.Status.OutputResources = []rpv1.OutputResource{}

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0).Return(nil)

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.NoError(t, err)
	})
}

func Test_getEnvOptions_PublicEndpointOverride(t *testing.T) {
	ctx := testcontext.New(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, nil, nil, nil}

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
	ctx := testcontext.New(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, nil, nil}

	t.Run("Get recipe data from connected mongoDB resources", func(t *testing.T) {
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)

		depId, _ := resources.ParseResource("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Datastores/mongoDatabases/test-mongo")
		mongoResource := buildMongoDBWithRecipe()
		mongoResource.PortableResourceMetadata.RecipeData = portableresources.RecipeData{}
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
	ctx := testcontext.New(t)

	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, nil, nil, nil}

	t.Run("Get secrets from recipe data when resource has associated recipe", func(t *testing.T) {
		mongoResource := buildMongoDBResourceDataWithRecipeAndSecrets()

		secret := "mongodb://testUser:testPassword@testAccount1.mongo.cosmos.azure.com:10255/db?ssl=true"
		mongoResource.SecretValues[pr_renderers.ConnectionStringValue] = rpv1.SecretValueReference{Value: secret}
		secretValues, err := dp.FetchSecrets(ctx, mongoResource)
		require.NoError(t, err)
		require.Equal(t, 1, len(secretValues))
		require.Equal(t, secret, secretValues[pr_renderers.ConnectionStringValue])
	})
}
