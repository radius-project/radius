// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/db"
	"github.com/project-radius/radius/pkg/corerp/renderers"

	"github.com/project-radius/radius/pkg/corerp/model"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateContainer)(nil)

var (
	// AsyncPutContainerOperationTimeout is the default timeout duration of async put container operation.
	AsyncPutContainerOperationTimeout = time.Duration(120) * time.Second
)

// CreateOrUpdateContainer is the controller implementation to create or update a container resource.
type CreateOrUpdateContainer struct {
	ctrl.BaseController
}

// NewCreateOrUpdateContainer creates a new CreateOrUpdateContainer.
func NewCreateOrUpdateContainer(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &CreateOrUpdateContainer{ctrl.NewBaseController(ds, sm)}, nil
}

// Run executes CreateOrUpdateContainer operation.
func (e *CreateOrUpdateContainer) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	existingResource := &datamodel.ContainerResource{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	exists := true
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		exists = false
	}

	if req.Method == http.MethodPatch && !exists {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	if exists && !existingResource.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(OngoingAsyncOperationOnResourceMessage), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	enrichMetadata(ctx, existingResource, newResource)

	// save start
	b, err := json.Marshal(newResource.Properties)
	if err != nil {
		return nil, err
	}

	var properties map[string]interface{}
	err = json.Unmarshal(b, &properties)
	if err != nil {
		return nil, err
	}

	item := db.NewDBRadiusResource(newResource.ID, properties)
	item.ProvisioningState = string(rest.DeployingStatus)

	obj, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), item, etag)
	if err != nil {
		return nil, err
	}
	// save end

	// deploy and render - start
	dp := deployment.NewDeploymentProcessor(model.ApplicationModel{}, nil, nil, nil)
	rendererOutput, err := dp.Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := dp.Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	deployedOuputResources := deploymentOutput.DeployedOutputResources
	var outputResources []map[string]interface{}
	for _, deployedOutputResource := range deployedOuputResources {
		outputResource := map[string]interface{}{
			deployedOutputResource.LocalID: deployedOutputResource,
		}
		outputResources = append(outputResources, outputResource)
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = outputResources
	newResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	newResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	// create db resource
	var dbdeployedOutputResources []db.OutputResource
	for _, deployedOutputResource := range deployedOuputResources {
		// Build database resource - copy updated properties to Resource field
		dbOutputResource := db.OutputResource{
			LocalID:      deployedOutputResource.LocalID,
			ResourceType: deployedOutputResource.ResourceType,
			Identity:     deployedOutputResource.Identity,
			Status: db.OutputResourceStatus{
				ProvisioningState:        db.Provisioned,
				ProvisioningErrorDetails: "",
			},
		}
		dbdeployedOutputResources = append(dbdeployedOutputResources, dbOutputResource)
	}

	resourceStatus := db.RadiusResourceStatus{
		ProvisioningState: db.Provisioned,
		OutputResources:   dbdeployedOutputResources,
	}

	b1, err := json.Marshal(newResource.Properties)
	if err != nil {
		return nil, err
	}

	var properties1 map[string]interface{}
	err = json.Unmarshal(b1, &properties1)
	if err != nil {
		return nil, err
	}

	deployedRadiusResource := db.RadiusResource{
		ID:             newResource.ID,
		Definition:     properties1,
		ComputedValues: deploymentOutput.ComputedValues,
		SecretValues:   convertSecretValues(deploymentOutput.SecretValues),

		Status: resourceStatus,

		ProvisioningState: string(rest.SuccededStatus),
	}

	obj1, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), deployedRadiusResource, obj.ETag)
	if err != nil {
		return nil, err
	}

	// end
	err = e.AsyncOperation.QueueAsyncOperation(ctx, serviceCtx, AsyncPutContainerOperationTimeout)
	if err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, obj1.ETag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	respCode := http.StatusCreated
	if req.Method == http.MethodPatch {
		respCode = http.StatusAccepted
	}

	return rest.NewAsyncOperationResponse(newResource, newResource.TrackedResource.Location, respCode, serviceCtx.ResourceID, serviceCtx.OperationID), nil
}

func convertSecretValues(input map[string]renderers.SecretValueReference) map[string]db.SecretValueReference {
	output := map[string]db.SecretValueReference{}
	for k, v := range input {
		output[k] = db.SecretValueReference{
			LocalID:       v.LocalID,
			Action:        v.Action,
			ValueSelector: v.ValueSelector,
			Value:         to.StringPtr(v.Value),
			Transformer:   v.Transformer,
		}
	}

	return output
}

// Validate extracts versioned resource from request and validate the properties.
func (e *CreateOrUpdateContainer) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.ContainerResource, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.ContainerDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)

	return dm, err
}

// enrichMetadata updates necessary metadata of the resource.
func enrichMetadata(ctx context.Context, er *datamodel.ContainerResource, nr *datamodel.ContainerResource) {
	sc := servicecontext.ARMRequestContextFromContext(ctx)

	nr.SystemData = ctrl.UpdateSystemData(er.SystemData, *sc.SystemData())

	if er.CreatedAPIVersion != "" {
		nr.CreatedAPIVersion = er.CreatedAPIVersion
	}

	nr.TenantID = sc.HomeTenantID

	nr.Properties.ProvisioningState = v1.ProvisioningStateAccepted
}
