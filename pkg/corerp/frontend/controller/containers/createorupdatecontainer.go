// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/resources"
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
	if exists && !existingResource.Properties.ProvisioningState.IsTerminal() {
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

	err = e.AsyncOperation.QueueAsyncOperation(ctx, serviceCtx, AsyncPutContainerOperationTimeout)
	if err != nil {
		rbErr := e.RollbackChanges(ctx, exists, existingResource, newResource, etag)
		if rbErr != nil {
			// TODO: Should this be an internal error response?
			return nil, rbErr
		}

		// TODO: Should this be an internal error response?
		return nil, err
	}

	locationHeader, err := getPath(serviceCtx.ResourceID, "operationResults", serviceCtx.OperationID)
	if err != nil {
		return nil, err
	}

	azureAsyncOpHeader, err := getPath(serviceCtx.ResourceID, "operationStatuses", serviceCtx.OperationID)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Location":             locationHeader,
		"Azure-AsyncOperation": azureAsyncOpHeader,
	}

	return rest.NewAsyncOperationCreatedResponse(newResource, headers), nil
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

// RollbackChanges function overwrites the object with an older copy or updates the state of the new object to Failed.
func (e *CreateOrUpdateContainer) RollbackChanges(ctx context.Context, exists bool, oldCopy *datamodel.ContainerResource, newCopy *datamodel.ContainerResource, etag string) error {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	cntr := oldCopy
	if !exists {
		cntr = newCopy
		cntr.Properties.ProvisioningState = v1.ProvisioningStateFailed
	}

	_, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), cntr, etag)

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

	// Accepted state
	// Q is going to update it to Updating
	// Once finished, Q will update it to a Terminal State
	// Update worker.go
	// No retry to add the message to the Q
	nr.Properties.ProvisioningState = v1.ProvisioningStateAccepted
}

// getPath returns the path for the given resource type.
func getPath(resourceID resources.ID, resourceType string, operationID uuid.UUID) (string, error) {
	var sb strings.Builder

	parsedID, err := resources.Parse(resourceID.String())
	if err != nil {
		return "", err
	}
	sb.WriteString(parsedID.RootScope())

	provider := parsedID.ProviderNamespace()
	if provider == "" {
		return "", errors.New("provider can not be empty string")
	}
	sb.WriteString(fmt.Sprintf("/providers/%s", provider))

	location := parsedID.FindScope(resources.LocationsSegment)
	if location != "" {
		sb.WriteString(fmt.Sprintf("/locations/%s", location))
	}

	sb.WriteString(fmt.Sprintf("/%s/%s", resourceType, operationID.String()))
	return sb.String(), nil
}
