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
	"github.com/project-radius/radius/pkg/rp"
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
			{
				ResourceType: resourcemodel.ResourceType{
					Type:     resourcekinds.Service,
					Provider: providers.ProviderKubernetes,
				},
				ResourceHandler: resourceHandler,
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
			LocalID: outputresource.LocalIDService,
			ResourceType: resourcemodel.ResourceType{
				Type:     resourcekinds.Service,
				Provider: providers.ProviderKubernetes,
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

	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}
	t.Run("verify render success", func(t *testing.T) {
		testResource := getTestResource()
		testRendererOutput := getTestRendererOutput()
		resourceID := getTestResourceID(testResource.ID)

		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
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
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		rendererOutput, err := dp.Render(ctx, resourceID, testResource)
		require.NoError(t, err)
		require.Equal(t, len(testRendererOutput.Resources), len(rendererOutput.Resources))
	})

	t.Run("verify render error", func(t *testing.T) {
		testResource := getTestResource()
		resourceID := getTestResourceID(testResource.ID)
		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render the resource"))
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
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

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
		depId1, _ := resources.Parse("/subscriptions/test-subscription/resourceGroups/test-resource-group/providers/Applications.Core/httpRoutes/A")
		requiredResources := []resources.ID{depId1}

		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(requiredResources, nil, nil)
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
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
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)

		_, err := dp.Render(ctx, resourceID, testResource)
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
		mocks.dbProvider.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Times(1).Return(mocks.db, nil)
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
		mocks.db.EXPECT().Get(gomock.Any(), gomock.Any()).Times(1).Return(&nr, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(testRendererOutput, nil)

		_, err := dp.Render(ctx, resourceID, testResource)
		require.Error(t, err, "provider unknown is not configured. Cannot support resource type azure.roleassignment")
	})
}

func Test_Deploy(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)
	dp := deploymentProcessor{mocks.model, mocks.dbProvider, mocks.secretsValueClient, nil}

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
				Provider: providers.ProviderKubernetes,
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
