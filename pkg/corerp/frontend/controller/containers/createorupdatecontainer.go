// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateContainer)(nil)

var ()

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
		// TODO: Should this be an validation error response?
		return nil, err
	}

	existingResource := &datamodel.ContainerResource{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		// TODO: Should this be an internal error response?
		return nil, err
	}

	exists := true
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		exists = false
	}

	// If this is a PATCH request but the resource doesn't exist
	if req.Method == http.MethodPatch && !exists {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	// If the resource exists and also not in a terminal state
	if exists && !v1.IsTerminalState(existingResource.Properties.ProvisioningState) {
		return rest.NewConflictResponse(ErrOngoingAsyncOperationOnResource.Error()), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		// TODO: Are we going to have ETag on Async requests?
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	updateExistingResourceData(ctx, existingResource, newResource)

	_, err = e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		// TODO: Should this be an internal error response?
		return nil, err
	}

	err = e.AsyncOperation.QueueAsyncOperation(ctx, serviceCtx, 60)
	if err != nil {
		rbErr := e.RollbackChanges(ctx, exists, existingResource, etag)
		if rbErr != nil {
			// TODO: Should this be an internal error response?
			return nil, err
		}

		// TODO: Should this be an internal error response?
		return nil, err
	}

	locationHeader, err := getHeaderPath(serviceCtx.ResourceID.String(), "operationResults", serviceCtx.OperationID.String())
	if err != nil {
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Message: err.Error(),
			},
		}), nil
	}

	azureAsyncOpHeader, err := getHeaderPath(serviceCtx.ResourceID.String(), "operationStatuses", serviceCtx.OperationID.String())
	if err != nil {
		return rest.NewInternalServerErrorARMResponse(armerrors.ErrorResponse{
			Error: armerrors.ErrorDetails{
				Message: err.Error(),
			},
		}), nil
	}

	headers := map[string]string{
		"Location":             locationHeader.String(),
		"Azure-AsyncOperation": azureAsyncOpHeader.String(),
	}

	return rest.NewAsyncOperationCreatedResponse(newResource.Properties, headers), nil
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

func (e *CreateOrUpdateContainer) RollbackChanges(ctx context.Context, exists bool, oldCnt *datamodel.ContainerResource, etag string) error {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	var err error

	// If the object existed before, overwrite with the older copy
	if exists {
		_, err = e.SaveResource(ctx, serviceCtx.ResourceID.String(), oldCnt, etag)
	} else {
		err = e.DataStore.Delete(ctx, serviceCtx.ResourceID.String())
	}

	if err != nil {
		return err
	}

	return nil
}

// updateExistingResourceData updates the container resource before it is saved to the DB.
func updateExistingResourceData(ctx context.Context, er *datamodel.ContainerResource, nr *datamodel.ContainerResource) {
	sc := servicecontext.ARMRequestContextFromContext(ctx)

	nr.SystemData = ctrl.UpdateSystemData(er.SystemData, *sc.SystemData())

	if er.CreatedAPIVersion != "" {
		nr.CreatedAPIVersion = er.CreatedAPIVersion
	}

	nr.TenantID = sc.HomeTenantID

	nr.Properties.ProvisioningState = v1.ProvisioningStateUpdating
}

func GetOperationStatusPath(r *http.Request, id string) string {
	vars := mux.Vars(r)

	return "/subscriptions/" + vars["subscriptionID"] + "/providers/Applications.Core/locations/" + vars["location"] + "/operationsStatuses/" + id
}

func GetOperationResultPath(r *http.Request, id string) string {
	vars := mux.Vars(r)

	return "/subscriptions/" + vars["subscriptionID"] + "/providers/Applications.Core/locations/" + vars["location"] + "/operationsResults/" + id
}

// getHeaderPath function
func getHeaderPath(resourceID string, resourceType string, operationID string) (resources.ID, error) {
	id, err := resources.Parse(resourceID)
	if err != nil {
		return id, err
	}

	ts := resources.TypeSegment{
		Type: resourceType,
		Name: operationID,
	}

	id = id.Truncate().Append(ts)

	return id, nil
}
