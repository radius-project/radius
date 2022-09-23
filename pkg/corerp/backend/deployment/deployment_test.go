// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	connectorrp_dm "github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
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

type SharedMocks struct {
	model              model.ApplicationModel
	db                 *store.MockStorageClient
	dbProvider         *dataprovider.MockDataStorageProvider
	resourceHandler    *handlers.MockResourceHandler
	renderer           *renderers.MockRenderer
	secretsValueClient *rp.MockSecretValueClient
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
		secretsValueClient: rp.NewMockSecretValueClient(ctrl),
	}
}

func getTestResource() datamodel.ContainerResource {
	rawDataModel := radiustesting.ReadFixture("containerresourcedatamodel.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getLowerCaseTestResource() datamodel.ContainerResource {
	rawDataModel := radiustesting.ReadFixture("containerresourcedatamodellowercase.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getUpperCaseTestResource() datamodel.ContainerResource {
	rawDataModel := radiustesting.ReadFixture("containerresourcedatamodeluppercase.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getTestRendererOutput() renderers.RendererOutput {
	testOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDService,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: resourcemodel.ProviderKubernetes,
			},
		},
	}

	rendererOutput := renderers.RendererOutput{
		Resources: testOutputResources,
		ComputedValues: map[string]rp.ComputedValueReference{
			"url": {
				Value: "http://test-application/test-route:8080",
			},
		},
	}
	return rendererOutput
}

func getTestResourceID(id string) resources.ID {
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

func Test_Render(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil, nil}
	t.Run("verify render success", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		depId2, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Connector/mongoDatabases/test-mongo")
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
				BasicResourceProperties: rp.BasicResourceProperties{
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
		environment := datamodel.Environment{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
			Properties: datamodel.EnvironmentProperties{
				Compute: datamodel.EnvironmentCompute{
					KubernetesCompute: datamodel.KubernetesComputeProperties{
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
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
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

		mongoResource := connectorrp_dm.MongoDatabase{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Connector/mongoDatabases/test-mongo",
			},
			Properties: connectorrp_dm.MongoDatabaseProperties{
				MongoDatabaseResponseProperties: connectorrp_dm.MongoDatabaseResponseProperties{
					BasicResourceProperties: rp.BasicResourceProperties{
						Environment: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
					},
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

		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
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
				BasicResourceProperties: rp.BasicResourceProperties{
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
		environment := datamodel.Environment{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
			Properties: datamodel.EnvironmentProperties{
				Compute: datamodel.EnvironmentCompute{
					KubernetesCompute: datamodel.KubernetesComputeProperties{
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
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
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

		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
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
				BasicResourceProperties: rp.BasicResourceProperties{
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
		environment := datamodel.Environment{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
			Properties: datamodel.EnvironmentProperties{
				Compute: datamodel.EnvironmentCompute{
					KubernetesCompute: datamodel.KubernetesComputeProperties{
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
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
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
		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
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
				BasicResourceProperties: rp.BasicResourceProperties{
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
		environment := datamodel.Environment{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
			Properties: datamodel.EnvironmentProperties{
				Compute: datamodel.EnvironmentCompute{
					KubernetesCompute: datamodel.KubernetesComputeProperties{
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
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
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
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\" does not exist", err.(*conv.ErrClientRP).Message)
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
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "application ID \"invalid-app-id\" for the resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\" is not a valid id. Error: 'invalid-app-id' is not a valid resource id", err.(*conv.ErrClientRP).Message)
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
		require.Equal(t, v1.CodeInvalid, err.(*conv.ErrClientRP).Code)
		require.Equal(t, "linked application ID \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/app/test-application\" for resource \"/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/containers/test-resource\" has invalid application resource type.", err.(*conv.ErrClientRP).Message)
	})

	t.Run("Missing output resource provider", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Provider = ""
		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
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
				BasicResourceProperties: rp.BasicResourceProperties{
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
		environment := datamodel.Environment{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
			Properties: datamodel.EnvironmentProperties{
				Compute: datamodel.EnvironmentCompute{
					KubernetesCompute: datamodel.KubernetesComputeProperties{
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
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
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
		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
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
				BasicResourceProperties: rp.BasicResourceProperties{
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
		environment := datamodel.Environment{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/environments/env0",
				},
			},
			Properties: datamodel.EnvironmentProperties{
				Compute: datamodel.EnvironmentCompute{
					KubernetesCompute: datamodel.KubernetesComputeProperties{
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
		httprouteA := datamodel.HTTPRoute{
			BaseResource: v1.BaseResource{
				TrackedResource: v1.TrackedResource{
					ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
				},
			},
			Properties: &datamodel.HTTPRouteProperties{
				BasicResourceProperties: rp.BasicResourceProperties{
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

		expectedKubernetesproperties := map[string]string{
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
				Name:       expectedKubernetesproperties[handlers.ResourceName],
				Namespace:  expectedKubernetesproperties[handlers.KubernetesNamespaceKey],
				Kind:       expectedKubernetesproperties[handlers.KubernetesKindKey],
				APIVersion: expectedKubernetesproperties[handlers.ResourceName],
			},
		}

		mocks.resourceHandler.EXPECT().GetResourceIdentity(gomock.Any(), gomock.Any()).Times(1).Return(expectedIdentity, nil)
		mocks.resourceHandler.EXPECT().GetResourceNativeIdentityKeyProperties(gomock.Any(), gomock.Any()).Times(1).Return(expectedKubernetesproperties, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		deploymentOutput, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(deploymentOutput.DeployedOutputResources))
		require.NotEqual(t, resourcemodel.ResourceIdentity{}, deploymentOutput.DeployedOutputResources[0].Identity)
		require.Equal(t, map[string]interface{}{"url": testRendererOutput.ComputedValues["url"].Value}, deploymentOutput.ComputedValues)
	})

	t.Run("Verify deploy failure", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().GetResourceIdentity(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to deploy the resource"))

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
	})

	t.Run("Output resource dependency missing local ID", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].Dependencies = []outputresource.Dependency{
			{LocalID: ""},
		}

		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "missing localID for outputresource", err.Error())
	})

	t.Run("Invalid output resource type", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Type = "foo"

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "output resource kind 'Provider: kubernetes, Type: foo' is unsupported", err.Error())
	})

	t.Run("Missing output resource identity", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		mocks.resourceHandler.EXPECT().GetResourceIdentity(gomock.Any(), gomock.Any()).Times(1).Return(resourcemodel.ResourceIdentity{}, nil)
		mocks.resourceHandler.EXPECT().GetResourceNativeIdentityKeyProperties(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)

		_, err := dp.Deploy(ctx, resourceID, testRendererOutput)
		require.Error(t, err)
		require.Equal(t, "output resource \"Service\" does not have an identity. This is a bug in the handler", err.Error())
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
		testResource.Properties.Status.OutputResources = []outputresource.OutputResource{}

		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(0).Return(nil)

		err := dp.Delete(ctx, resourceID, testResource.Properties.Status.OutputResources)
		require.NoError(t, err)
	})
}

func Test_getEnvOptions_PublicEndpointOverride(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, nil, nil, nil, nil}

	radiusSystemNamespace := "radius-system"

	t.Run("Verify getEnvOptions succeeds (host:port)", func(t *testing.T) {
		os.Setenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE", "localhost:8000")
		defer os.Unsetenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

		options, err := dp.getEnvOptions(ctx, radiusSystemNamespace)
		require.NoError(t, err)

		require.True(t, options.Gateway.PublicEndpointOverride)
		require.Equal(t, options.Gateway.Hostname, "localhost")
		require.Equal(t, options.Gateway.Port, "8000")
		require.Equal(t, options.Gateway.ExternalIP, "")
	})

	t.Run("Verify getEnvOptions succeeds (host)", func(t *testing.T) {
		os.Setenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE", "www.contoso.com")
		defer os.Unsetenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

		options, err := dp.getEnvOptions(ctx, radiusSystemNamespace)
		require.NoError(t, err)

		require.True(t, options.Gateway.PublicEndpointOverride)
		require.Equal(t, options.Gateway.Hostname, "www.contoso.com")
		require.Equal(t, options.Gateway.Port, "")
		require.Equal(t, options.Gateway.ExternalIP, "")
	})

	t.Run("Verify getEnvOptions fails (URL)", func(t *testing.T) {
		os.Setenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE", "http://localhost:8000")
		defer os.Unsetenv("RADIUS_PUBLIC_ENDPOINT_OVERRIDE")

		options, err := dp.getEnvOptions(ctx, radiusSystemNamespace)
		require.Error(t, err)
		require.EqualError(t, err, "a URL is not accepted here. Please reinstall Radius with a valid public endpoint using rad install kubernetes --reinstall --public-endpoint-override <your-endpoint>")
		require.Equal(t, options, renderers.EnvironmentOptions{})
	})
}
