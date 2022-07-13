// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"errors"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
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
func NewCreateOrUpdateContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateContainer{ctrl.NewBaseController(opts)}, nil
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
		return rest.NewConflictResponse(controller.OngoingAsyncOperationOnResourceMessage), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	enrichMetadata(ctx, existingResource, newResource)

	obj, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	err = e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncPutContainerOperationTimeout)
	if err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, obj.ETag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	respCode := http.StatusCreated
	if req.Method == http.MethodPatch {
		respCode = http.StatusAccepted
	}

	return rest.NewAsyncOperationResponse(newResource, newResource.TrackedResource.Location, respCode,
		serviceCtx.ResourceID, serviceCtx.OperationID, serviceCtx.APIVersion), nil
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
