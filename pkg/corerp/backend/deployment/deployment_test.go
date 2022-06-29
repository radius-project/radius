// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	connectorrp_renderer "github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/pkg/resourcemodel"
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
	secretsValueClient *renderers.MockSecretValueClient
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
					Provider: providers.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.Secret,
					Provider: providers.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
			},
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.AzureRoleAssignment,
					Provider: providers.ProviderAzure,
				},
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
		resourceHandler:    resourceHandler,
		renderer:           renderer,
		secretsValueClient: renderers.NewMockSecretValueClient(ctrl),
	}
}

func getTestResource() datamodel.ContainerResource {
	rawDataModel := radiustesting.ReadFixture("containerresourcedatamodel.json")
	testResource := &datamodel.ContainerResource{}
	_ = json.Unmarshal(rawDataModel, testResource)
	return *testResource
}

func getTestRendererOutput() renderers.RendererOutput {
	testOutputResources := []outputresource.OutputResource{
		{
			LocalID: outputresource.LocalIDDeployment,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.AzureRoleAssignment,
				Provider: providers.ProviderAzure,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.AzureRoleAssignment,
					Provider: providers.ProviderAzure,
				},
				Data: resourcemodel.ARMIdentity{},
			},
		},
		{
			LocalID: outputresource.LocalIDService,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Deployment,
				Provider: providers.ProviderKubernetes,
			},
			Identity: resourcemodel.ResourceIdentity{
				ResourceType: &resourcemodel.ResourceType{
					Type:     resourcekinds.Deployment,
					Provider: providers.ProviderKubernetes,
				},
				Data: resourcemodel.KubernetesIdentity{},
			},
		},
	}

	rendererOutput := renderers.RendererOutput{
		Resources: testOutputResources,
		SecretValues: map[string]renderers.SecretValueReference{
			connectorrp_renderer.ConnectionStringValue: {
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
			connectorrp_renderer.DatabaseNameValue: {
				Value: "test-database",
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

	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}
	t.Run("verify render success", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)

		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		depId2, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/B")
		radiusResourceIDs := []resources.ID{depId1, depId2}

		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Return(radiusResourceIDs, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(mocks.db, nil)
		httprouteA := datamodel.HTTPRoute{
			TrackedResource: v1.TrackedResource{
				ID: "/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A",
			},
			Properties: &datamodel.HTTPRouteProperties{},
		}
		nr := store.Object{
			Metadata: store.Metadata{
				ID: httprouteA.ID,
			},
			Data: httprouteA,
		}
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&nr, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render error", func(t *testing.T) {
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render the resource"))

		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)

		_, err := dp.Render(ctx, resourceID, testResource)
		require.Error(t, err, "failed to render the resource")
	})

	t.Run("Invalid resource type", func(t *testing.T) {
		testInvalidResourceID := "/subscriptions/test-sub/resourceGroups/test-group/providers/Applications.foo/foo/foo"
		testResource := getTestResource()
		resourceID := getTestResourceID(testInvalidResourceID)

		_, err := dp.Render(ctx, resourceID, testResource)
		require.Error(t, err, "radius resource type 'Applications.foo/foo' is unsupported")

	})

	t.Run("Missing output resource provider", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Provider = ""

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)

		_, err := dp.Render(ctx, resourceID, testResource)
		require.Error(t, err, "output resource \"AzureCosmosAccount\" does not have a provider specified")
	})

	t.Run("Unsupported output resource provider", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		testRendererOutput.Resources[0].ResourceType.Provider = "unknown"

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)

		_, err := dp.Render(ctx, resourceID, testResource)
		require.Error(t, err, "provider unknown is not configured. Cannot support resource type azure.cosmosdb.account")
	})
}
