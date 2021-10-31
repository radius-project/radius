// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deployment

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armerrors"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const (
	radiusProviderName  = "radiusv3"
	testApplicationName = "test-application"
	subscriptionID      = "test-subscription"
	resourceGroup       = "test-resource-group"
	resourceName        = "test-resource"
)

var (
	testAzureID    = fullyQualifiedAzureID(containerv1alpha3.ResourceType, resourceName)
	testResourceID = getResourceID(testAzureID)

	testDBOutputResources = []db.OutputResource{
		{
			LocalID:      outputresource.LocalIDDeployment,
			ResourceKind: resourcekinds.Kubernetes,
			Managed:      true,
		},
		{
			LocalID:      outputresource.LocalIDService,
			ResourceKind: resourcekinds.Kubernetes,
			Managed:      true,
		},
	}

	testRadiusResource = db.RadiusResource{
		ID:              testAzureID,
		Type:            testResourceID.Type(),
		SubscriptionID:  subscriptionID,
		ResourceGroup:   resourceGroup,
		ApplicationName: testApplicationName,
		ResourceName:    resourceName,
		Definition: map[string]interface{}{
			"data": true,
		},
		ProvisioningState: string(rest.SuccededStatus),
		Status: db.RadiusResourceStatus{
			OutputResources: testDBOutputResources,
		},
	}
)

type SharedMocks struct {
	model              model.ApplicationModel
	db                 *db.MockRadrpDB
	resourceHandler    *handlers.MockResourceHandler
	healthHandler      *handlers.MockHealthHandler
	renderer           *renderers.MockRenderer
	secretsValueClient *renderers.MockSecretValueClient
}

func setup(t *testing.T) SharedMocks {
	ctrl := gomock.NewController(t)

	renderer := renderers.NewMockRenderer(ctrl)
	resourceHandler := handlers.NewMockResourceHandler(ctrl)
	healthHandler := handlers.NewMockHealthHandler(ctrl)

	// NOTE: right now these tests have some reliance on the Kubernetes-based logic for whether a resource is monitored by
	// the health system.
	skipHealthCheckKubernetesKinds := map[string]bool{
		resourcekinds.Service: true,
		resourcekinds.Secret:  true,
		resourcekinds.Ingress: true,
	}
	model := model.NewModel(
		[]model.RadiusResourceModel{
			{
				ResourceType: containerv1alpha3.ResourceType,
				Renderer:     renderer,
			},
		},
		[]model.OutputResourceModel{
			{
				Kind:            resourcekinds.Kubernetes,
				HealthHandler:   healthHandler,
				ResourceHandler: resourceHandler,
				// We can monitor specific kinds of Kubernetes resources for health tracking, but not all of them.
				ShouldSupportHealthMonitorFunc: func(identity resourcemodel.ResourceIdentity) bool {
					if identity.Kind == resourcemodel.IdentityKindKubernetes {
						skip := skipHealthCheckKubernetesKinds[identity.Data.(resourcemodel.KubernetesIdentity).Kind]
						return !skip
					}

					return false
				},
			},
		})

	return SharedMocks{
		model:              model,
		db:                 db.NewMockRadrpDB(ctrl),
		resourceHandler:    resourceHandler,
		healthHandler:      healthHandler,
		renderer:           renderer,
		secretsValueClient: renderers.NewMockSecretValueClient(ctrl),
	}
}

func getResourceID(azureID string) azresources.ResourceID {
	resourceID, err := azresources.Parse(azureID)
	if err != nil {
		panic(err)
	}

	return resourceID
}

func fullyQualifiedAzureID(resourceType string, resourceName string) string {
	return azresources.MakeID(subscriptionID, resourceGroup,
		azresources.ResourceType{Type: azresources.CustomProvidersResourceProviders, Name: radiusProviderName},
		azresources.ResourceType{Type: azresources.ApplicationResourceType, Name: testApplicationName},
		azresources.ResourceType{Type: resourceType, Name: resourceName})
}

func createContext(t *testing.T) context.Context {
	logger, err := radlogger.NewTestLogger(t)
	if err != nil {
		t.Log("Unable to initialize logger")
		return context.Background()
	}
	return logr.NewContext(context.Background(), logger)
}

func Test_DeployExistingResource_Success(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mocks.secretsValueClient, nil}

	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})
	expectedDependencyIDs := []azresources.ResourceID{
		getResourceID(fullyQualifiedAzureID("HttpRoute", "A")),
		getResourceID(fullyQualifiedAzureID("HttpRoute", "B")),
	}

	testOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDeployment,
		ResourceKind: resourcekinds.Kubernetes,
		Deployed:     false,
		Managed:      true,
		Identity: resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Name:      resourceName,
				Namespace: testApplicationName,
			},
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
	}

	mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(expectedDependencyIDs, nil)
	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(2).Return(db.RadiusResource{}, nil)
	mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any()).Times(1).Return(rendererOutput, nil)
	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(testRadiusResource, nil)
	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)
	mocks.healthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(1).Return(healthcontract.HealthCheckOptions{})
	mocks.db.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)

	// Validate registration of the output resource
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionRegister, msg1.Action)
	require.Equal(t, testRadiusResource.ID, msg1.Resource.RadiusResourceID)
	require.Equal(t, testOutputResource.ResourceKind, msg1.Resource.ResourceKind)
}

func Test_DeployNewResource_Success(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}

	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})

	mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
	mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
	// validates ErrNotFound is ignored
	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, db.ErrNotFound)
	mocks.db.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)
}

// Validates operation update is called after failure at any step to set the operation status to failed
func Test_DeployFailure_OperationUpdated(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}

	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})

	t.Run("verify get dependencies failure", func(t *testing.T) {
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, errors.New("failed to get dependencies"))
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
		mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

		err := dp.Deploy(ctx, operationID, testRadiusResource)
		require.Error(t, err, "failed to get dependencies")
	})

	t.Run("verify database get resource failure", func(t *testing.T) {
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, errors.New("failed to get the resource from database"))
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
		mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

		err := dp.Deploy(ctx, operationID, testRadiusResource)
		require.Error(t, err, "failed to get the resource from database")
	})

	t.Run("verify database update resource status failure", func(t *testing.T) {
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(testRadiusResource, nil)
		mocks.db.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to update resource status in the database"))
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
		mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

		err := dp.Deploy(ctx, operationID, testRadiusResource)
		require.Error(t, err, "failed to update resource status in the database")
	})
}

func Test_Render_InvalidResourceTypeErr(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}

	azureID := fullyQualifiedAzureID("foo", resourceName)
	resourceID := getResourceID(azureID)
	radiusResource := db.RadiusResource{
		ID:                azureID,
		Type:              resourceID.Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   testApplicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.SuccededStatus),
		Status:            db.RadiusResourceStatus{},
		Definition: map[string]interface{}{
			"data": true,
		},
	}

	_, armerr, err := dp.renderResource(ctx, resourceID, radiusResource)
	expectedArmErr := armerrors.ErrorDetails{
		Code:    armerrors.Invalid,
		Message: err.Error(),
		Target:  resourceID.ID,
	}
	require.Error(t, err, "resource kind 'foo' is unsupported")
	require.Equal(t, expectedArmErr, *armerr)
}

func Test_Render_DatabaseLookupInternalError(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}

	expectedDependencyIDs := []azresources.ResourceID{getResourceID(fullyQualifiedAzureID("HttpRoute", "A"))}

	mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(expectedDependencyIDs, nil)
	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, errors.New("failed to get resource from database"))

	_, armerr, err := dp.renderResource(ctx, testResourceID, testRadiusResource)
	expectedArmErr := armerrors.ErrorDetails{
		Code:    armerrors.Internal,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err, "failed to get resource from database")
	require.Equal(t, expectedArmErr, *armerr)
}

func Test_RendererFailure_InvalidError(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}

	mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
	mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render resource"))

	_, armerr, err := dp.renderResource(ctx, testResourceID, testRadiusResource)
	expectedArmErr := armerrors.ErrorDetails{
		Code:    armerrors.Invalid,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err, "failed to render resource")
	require.Equal(t, expectedArmErr, *armerr)
}

func Test_DeployRenderedResources_ComputedValues(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{ResourceRegistrationWithHealthChannel: registrationChannel}, mocks.secretsValueClient, nil}

	testOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDeployment,
		ResourceKind: resourcekinds.Kubernetes,
		Deployed:     true,
		Managed:      true,
		Identity: resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Kind:      resourcekinds.Kubernetes,
				Name:      "test-name",
				Namespace: "test-namespace",
			},
		},
		Resource: map[string]interface{}{
			"some-data": "jsonpointer-value",
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
		ComputedValues: map[string]renderers.ComputedValueReference{
			"test-key1": {
				LocalID: outputresource.LocalIDDeployment,
				Value:   "static-value",
			},
			"test-key2": {
				LocalID:           outputresource.LocalIDDeployment,
				PropertyReference: "property-key",
			},
			"test-key3": {
				LocalID:     outputresource.LocalIDDeployment,
				JSONPointer: "/some-data",
			},
		},
	}

	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)

	properties := map[string]string{"property-key": "property-value"}
	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(properties, nil)
	mocks.healthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(1).Return(healthcontract.HealthCheckOptions{})

	result, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	require.NoError(t, err)
	require.Nil(t, armerr)

	expected := map[string]interface{}{
		"test-key1": "static-value",
		"test-key2": "property-value",
		"test-key3": "jsonpointer-value",
	}
	require.Equal(t, expected, result.ComputedValues)
	<-registrationChannel
}

func Test_DeployRenderedResources_ErrorCodes(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}

	testOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDeployment,
		ResourceKind: resourcekinds.Kubernetes,
		Deployed:     false,
		Managed:      true,
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
	}

	t.Run("verify internal error for missing output resource identity", func(t *testing.T) {
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)

		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
		expectedArmErr := armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  testResourceID.ID,
		}
		require.Error(t, err, "output resource Kubernetes does not have an identity. This is a bug in the handler.")
		require.Equal(t, expectedArmErr, *armerr)
	})

	t.Run("verify no-op for database resource not found error", func(t *testing.T) {
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, db.ErrNotFound)

		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, renderers.RendererOutput{})
		require.NoError(t, err)
		require.Nil(t, armerr)
	})

	t.Run("verify internal error for non 404 database errors", func(t *testing.T) {
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, errors.New("failed to get resource from database"))

		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
		expectedArmErr := armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  testResourceID.ID,
		}
		require.Error(t, err)
		require.Equal(t, expectedArmErr, *armerr)
	})

	t.Run("verify internal error for handler put failure", func(t *testing.T) {
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, errors.New("handler put failure"))

		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
		expectedArmErr := armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  testResourceID.ID,
		}
		require.Error(t, err)
		require.Equal(t, expectedArmErr, *armerr)
	})

	t.Run("verify internal error for missing output resource localID", func(t *testing.T) {
		testOutputResource.Dependencies = []outputresource.Dependency{{LocalID: ""}}
		rendererOutput := renderers.RendererOutput{
			Resources: []outputresource.OutputResource{testOutputResource},
		}
		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
		expectedArmErr := armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  testResourceID.ID,
		}
		require.Error(t, err)
		require.Equal(t, expectedArmErr, *armerr)
	})

	t.Run("verify invalid JSON pointer in computed value", func(t *testing.T) {
		localTestOutputResource := outputresource.OutputResource{
			LocalID:      "test-local-id",
			ResourceKind: resourcekinds.Kubernetes,
			Identity: resourcemodel.ResourceIdentity{
				Kind: resourcemodel.IdentityKindKubernetes,
			},
			Deployed: true,
			Managed:  true,
		}
		localRendererOutput := renderers.RendererOutput{
			Resources: []outputresource.OutputResource{localTestOutputResource},
			ComputedValues: map[string]renderers.ComputedValueReference{
				"test-value": {
					LocalID:     "test-local-id",
					JSONPointer: ".ddkfkdk",
				},
			},
		}

		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)

		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, localRendererOutput)
		expectedArmErr := armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  testResourceID.ID,
		}
		require.Error(t, err)
		require.Equal(t, expectedArmErr, *armerr)
	})

	t.Run("verify JSON pointer in computed value has missing result in output", func(t *testing.T) {
		localTestOutputResource := outputresource.OutputResource{
			LocalID:      "test-local-id",
			ResourceKind: resourcekinds.Kubernetes,
			Identity: resourcemodel.ResourceIdentity{
				Kind: resourcemodel.IdentityKindKubernetes,
			},
			Deployed: true,
			Managed:  true,
			Resource: map[string]interface{}{
				"some-data": 3,
			},
		}
		localRendererOutput := renderers.RendererOutput{
			Resources: []outputresource.OutputResource{localTestOutputResource},
			ComputedValues: map[string]renderers.ComputedValueReference{
				"test-value": {
					LocalID:     "test-local-id",
					JSONPointer: "/some-other-data", // this key is missing
				},
			},
		}

		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)
		mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)

		_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, localRendererOutput)
		expectedArmErr := armerrors.ErrorDetails{
			Code:    armerrors.Internal,
			Message: err.Error(),
			Target:  testResourceID.ID,
		}
		require.Error(t, err)
		require.Equal(t, expectedArmErr, *armerr)
	})
}

func Test_Delete_Success(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mocks.secretsValueClient, nil}

	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})

	mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	mocks.db.EXPECT().DeleteV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Delete(ctx, operationID, testRadiusResource)
	require.NoError(t, err)

	// Remove both the output resources from health check
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg1.Action)
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg2.Action)
}

func Test_Delete_Error(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mocks.secretsValueClient, nil}
	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})

	t.Run("validate error on handler delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("handler delete failure"))
		// Validate operation record is updated in the database on failure
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
		mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

		err := dp.Delete(ctx, operationID, testRadiusResource)
		require.Error(t, err, "handler delete failure")
	})

	t.Run("validate error on database delete failure", func(t *testing.T) {
		mocks.resourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)
		mocks.db.EXPECT().DeleteV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete resource from db"))
		// Validate operation record is updated in the database on failure
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
		mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

		err := dp.Delete(ctx, operationID, testRadiusResource)
		require.Error(t, err, "failed to delete resource from db")

		// Remove both the output resources from health check
		msg1 := <-registrationChannel
		require.Equal(t, healthcontract.ActionUnregister, msg1.Action)
		msg2 := <-registrationChannel
		require.Equal(t, healthcontract.ActionUnregister, msg2.Action)
	})
}

func Test_Delete_InvalidResourceKindFailure(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mocks.secretsValueClient, nil}
	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})

	localTestResource := testRadiusResource
	localTestResource.Status.OutputResources = []db.OutputResource{
		{
			LocalID:      outputresource.LocalIDDeployment,
			ResourceKind: resourcekinds.Kubernetes,
			Managed:      true,
		},
		{
			LocalID:      outputresource.LocalIDService,
			ResourceKind: "foo",
			Managed:      true,
		},
	}

	mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Delete(ctx, operationID, localTestResource)
	require.Error(t, err, "resource kind 'foo' is unsupported")
}

// Test failure to update operation does not return error
func Test_UpdateOperationFailure_NoOp(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{}, mocks.secretsValueClient, nil}
	operationID := testResourceID.Append(azresources.ResourceType{Type: azresources.OperationResourceType, Name: uuid.New().String()})

	t.Run("verify database get operation failure", func(t *testing.T) {
		mocks.renderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(3).Return([]azresources.ResourceID{}, nil)
		mocks.renderer.EXPECT().Render(gomock.Any(), gomock.Any()).Times(3).Return(renderers.RendererOutput{}, nil)
		mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(3).Return(db.RadiusResource{}, nil)
		mocks.db.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(3).Return(nil)

		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("failed to get operation"))
		err := dp.Deploy(ctx, operationID, testRadiusResource)
		require.NoError(t, err)
	})

	t.Run("verify database get operation not found error", func(t *testing.T) {
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(nil, db.ErrNotFound)
		err := dp.Deploy(ctx, operationID, testRadiusResource)
		require.NoError(t, err)
	})

	t.Run("verify database patch operation failure", func(t *testing.T) {
		mocks.db.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
		mocks.db.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(false, errors.New("failed to patch operation"))
		err := dp.Deploy(ctx, operationID, testRadiusResource)
		require.NoError(t, err)
	})
}

func Test_Deploy_WithSkipHealthMonitoring(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mocks.secretsValueClient, nil}

	testOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDSecret,
		ResourceKind: resourcekinds.Kubernetes,
		Deployed:     false,
		Managed:      true,
		Identity: resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Kind:      resourcekinds.Secret,
				Name:      resourceName,
				Namespace: testApplicationName,
			},
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
	}

	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).AnyTimes().Return(db.RadiusResource{}, nil)
	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).AnyTimes().Return(testRadiusResource, nil)
	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)
	mocks.healthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(0).Return(healthcontract.HealthCheckOptions{})

	radResource, _, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	require.NoError(t, err)
	require.Equal(t, healthcontract.HealthStateNotApplicable, radResource.Status.OutputResources[0].Status.HealthState)

	// Validate registration of the output resource
	require.Zero(t, len(registrationChannel))
}

func Test_Deploy_WithHealthMonitoring(t *testing.T) {
	ctx := createContext(t)
	mocks := setup(t)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{mocks.model, mocks.db, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mocks.secretsValueClient, nil}

	testOutputResource := outputresource.OutputResource{
		LocalID:      outputresource.LocalIDDeployment,
		ResourceKind: resourcekinds.Kubernetes,
		Deployed:     false,
		Managed:      true,
		Identity: resourcemodel.ResourceIdentity{
			Kind: resourcemodel.IdentityKindKubernetes,
			Data: resourcemodel.KubernetesIdentity{
				Name:      resourceName,
				Namespace: testApplicationName,
			},
		},
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
	}

	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).AnyTimes().Return(db.RadiusResource{}, nil)
	mocks.db.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).AnyTimes().Return(testRadiusResource, nil)
	mocks.resourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)
	mocks.healthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(1).Return(healthcontract.HealthCheckOptions{})

	radResource, _, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	require.NoError(t, err)
	require.Equal(t, "", radResource.Status.OutputResources[0].Status.HealthState)

	// Validate registration of the output resource
	require.Equal(t, 1, len(registrationChannel))
}
