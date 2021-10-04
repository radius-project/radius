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
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
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
	testAzureID    = fulyQualifiedAzureID(containerv1alpha3.ResourceType, resourceName)
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
		Status: db.ComponentStatus{
			OutputResources: testDBOutputResources,
		},
	}
)

func getResourceID(azureID string) azresources.ResourceID {
	resourceID, err := azresources.Parse(azureID)
	if err != nil {
		panic(err)
	}

	return resourceID
}

func fulyQualifiedAzureID(resourceType string, resourceName string) string {
	return azresources.MakeID(subscriptionID, resourceGroup,
		azresources.ResourceType{Type: azresources.CustomProvidersResourceProviders, Name: radiusProviderName},
		azresources.ResourceType{Type: resources.V3ApplicationResourceType, Name: testApplicationName},
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
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)

	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}
	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})
	expectedDependencyIDs := []azresources.ResourceID{
		getResourceID(fulyQualifiedAzureID("HttpRoute", "A")),
		getResourceID(fulyQualifiedAzureID("HttpRoute", "B")),
	}

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(expectedDependencyIDs, nil)
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(2).Return(db.RadiusResource{}, nil)
	mockRenderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)
	mockDB.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)
}

func Test_DeployNewResource_Success(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{})
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}

	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
	mockRenderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
	// validates ErrNotFound is ignored
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, db.ErrNotFound)
	mockDB.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)
}

// Validates operation update is called after failure at any step to set the operation status to failed
func Test_DeployFailure_OperationUpdated(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}

	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, errors.New("failed to get dependencies"))
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Deploy(ctx, operationID, testRadiusResource)
	require.Error(t, err, "failed to get dependencies")

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
	mockRenderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, errors.New("failed to get the resource from database"))
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err = dp.Deploy(ctx, operationID, testRadiusResource)
	require.Error(t, err, "failed to get the resource from database")

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
	mockRenderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, nil)
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(testRadiusResource, nil)
	mockDB.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to update resource status in the database"))
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err = dp.Deploy(ctx, operationID, testRadiusResource)
	require.Error(t, err, "failed to update resource status in the database")
}

func Test_Render_InvalidResourceTypeErr(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}

	azureID := fulyQualifiedAzureID("foo", resourceName)
	resourceID := getResourceID(azureID)
	radiusResource := db.RadiusResource{
		ID:                azureID,
		Type:              resourceID.Type(),
		SubscriptionID:    subscriptionID,
		ResourceGroup:     resourceGroup,
		ApplicationName:   testApplicationName,
		ResourceName:      resourceName,
		ProvisioningState: string(rest.SuccededStatus),
		Status:            db.ComponentStatus{},
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
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}

	expectedDependencyIDs := []azresources.ResourceID{getResourceID(fulyQualifiedAzureID("HttpRoute", "A"))}

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return(expectedDependencyIDs, nil)
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, errors.New("failed to get resource from database"))

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
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(1).Return([]azresources.ResourceID{}, nil)
	mockRenderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(renderers.RendererOutput{}, errors.New("failed to render resource"))

	_, armerr, err := dp.renderResource(ctx, testResourceID, testRadiusResource)
	expectedArmErr := armerrors.ErrorDetails{
		Code:    armerrors.Invalid,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err, "failed to render resource")
	require.Equal(t, expectedArmErr, *armerr)
}

func Test_DeployRenderedResources_ErrorCodes(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)
	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}

	testOutputResource := outputresource.OutputResource{
		ResourceKind: resourcekinds.Kubernetes,
		Deployed:     false,
		Managed:      true,
		//*** Type:     outputresource.TypeKubernetes,
	}
	rendererOutput := renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
	}

	// Verify db resource not found error does not result into error // TODO Add back
	// mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, db.ErrNotFound)
	// mockResourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, nil)
	//*** mockHealthHandler.EXPECT().GetHealthOptions(gomock.Any()).Times(1).Return(healthcontract.HealthCheckOptions{})

	// _, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	// require.NoError(t, err)
	// require.Nil(t, armerr)

	// Verify an error to retreive resource from database other than not found should result into internal arm error
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, errors.New("failed to get resource from database"))

	_, armerr, err := dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	expectedArmErr := armerrors.ErrorDetails{
		Code:    armerrors.Internal,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err)
	require.Equal(t, expectedArmErr, *armerr)

	// Verify handler put failure translates into internal error
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)
	mockResourceHandler.EXPECT().Put(gomock.Any(), gomock.Any()).Times(1).Return(map[string]string{}, errors.New("handler put failure"))

	_, armerr, err = dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	expectedArmErr = armerrors.ErrorDetails{
		Code:    armerrors.Internal,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err)
	require.Equal(t, expectedArmErr, *armerr)

	// Verify missing output resource localID translates into internal error (- failure to order output resources)
	testOutputResource.Dependencies = []outputresource.Dependency{
		{
			LocalID: "",
		},
	}
	rendererOutput.Resources = []outputresource.OutputResource{testOutputResource}

	_, armerr, err = dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	expectedArmErr = armerrors.ErrorDetails{
		Code:    armerrors.Internal,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err)
	require.Equal(t, expectedArmErr, *armerr)

	// Verify invalid resource kind results into invalid arm error
	testOutputResource = outputresource.OutputResource{
		ResourceKind: "foo",
		Deployed:     false,
		Managed:      true,
		//*** Type:         outputresource.TypeKubernetes,
		Dependencies: []outputresource.Dependency{},
	}
	rendererOutput = renderers.RendererOutput{
		Resources: []outputresource.OutputResource{testOutputResource},
	}

	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(db.RadiusResource{}, nil)

	_, armerr, err = dp.deployRenderedResources(ctx, testResourceID, testRadiusResource, rendererOutput)
	expectedArmErr = armerrors.ErrorDetails{
		Code:    armerrors.Invalid,
		Message: err.Error(),
		Target:  testResourceID.ID,
	}
	require.Error(t, err)
	require.Equal(t, expectedArmErr, *armerr)
}

func Test_Delete_Success(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)

	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mockSecretsValueClient}

	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})

	mockResourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	mockDB.EXPECT().DeleteV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(nil)
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

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
	ctrl := gomock.NewController(t)
	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mockSecretsValueClient}
	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})

	// Handler Delete failure
	mockResourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("handler delete failure"))
	// Validate operation record is updated in the database on failure
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Delete(ctx, operationID, testRadiusResource)
	require.Error(t, err, "handler delete failure")

	// Database delete failure
	mockResourceHandler.EXPECT().Delete(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	mockDB.EXPECT().DeleteV3Resource(gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to delete resource from db"))
	// Validate operation record is updated in the database on failure
	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err = dp.Delete(ctx, operationID, testRadiusResource)
	require.Error(t, err, "failed to delete resource from db")

	// Remove both the output resources from health check
	msg1 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg1.Action)
	msg2 := <-registrationChannel
	require.Equal(t, healthcontract.ActionUnregister, msg2.Action)
}

func Test_Delete_InvalidResourceKindFailure(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)
	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)

	registrationChannel := make(chan healthcontract.ResourceHealthRegistrationMessage, 2)
	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{
		ResourceRegistrationWithHealthChannel: registrationChannel,
	}, mockSecretsValueClient}
	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})

	localTestResource := testRadiusResource
	localTestResource.Status.OutputResources = []db.OutputResource{
		{
			LocalID: outputresource.LocalIDDeployment,
			//*** OutputResourceType: outputresource.TypeKubernetes,
			ResourceKind: resourcekinds.Kubernetes,
			Managed:      true,
		},
		{
			LocalID: outputresource.LocalIDService,
			//*** OutputResourceType: outputresource.TypeKubernetes,
			ResourceKind: "foo",
			Managed:      true,
		},
	}

	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(true, nil)

	err := dp.Delete(ctx, operationID, localTestResource)
	require.Error(t, err, "resource kind 'foo' is unsupported")
}

// Test failure to update operation does not return error
func Test_UpdateOperationFailure_NoOp(t *testing.T) {
	ctx := createContext(t)
	ctrl := gomock.NewController(t)
	mockDB := db.NewMockRadrpDB(ctrl)
	mockResourceHandler := handlers.NewMockResourceHandler(ctrl)
	mockHealthHandler := handlers.NewMockHealthHandler(ctrl)
	mockRenderer := renderers.NewMockRenderer(ctrl)
	mockSecretsValueClient := renderers.NewMockSecretValueClient(ctrl)
	model := model.NewModelV3(map[string]renderers.Renderer{
		containerv1alpha3.ResourceType: mockRenderer,
	}, map[string]model.Handlers{
		resourcekinds.Kubernetes: {
			ResourceHandler: mockResourceHandler,
			HealthHandler:   mockHealthHandler,
		},
	},
		map[string]renderers.SecretValueTransformer{},
	)

	dp := deploymentProcessor{model, mockDB, &healthcontract.HealthChannels{}, mockSecretsValueClient}
	operationID := testResourceID.Append(azresources.ResourceType{Type: resources.V3OperationResourceType, Name: uuid.New().String()})

	mockRenderer.EXPECT().GetDependencyIDs(gomock.Any(), gomock.Any()).Times(3).Return([]azresources.ResourceID{}, nil)
	mockRenderer.EXPECT().Render(gomock.Any(), gomock.Any(), gomock.Any()).Times(3).Return(renderers.RendererOutput{}, nil)
	mockDB.EXPECT().GetV3Resource(gomock.Any(), gomock.Any()).Times(3).Return(db.RadiusResource{}, nil)
	mockDB.EXPECT().UpdateV3ResourceStatus(gomock.Any(), gomock.Any(), gomock.Any()).Times(3).Return(nil)

	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(nil, errors.New("failed to get operation"))
	err := dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)

	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(nil, db.ErrNotFound)
	err = dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)

	mockDB.EXPECT().GetOperationByID(gomock.Any(), gomock.Any()).Times(1).Return(&db.Operation{}, nil)
	mockDB.EXPECT().PatchOperationByID(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(false, errors.New("failed to patch operation"))
	err = dp.Deploy(ctx, operationID, testRadiusResource)
	require.NoError(t, err)
}
