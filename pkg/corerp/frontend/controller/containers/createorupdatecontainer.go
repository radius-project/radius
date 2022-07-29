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
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateContainer)(nil)

var (
	// AsyncPutContainerOperationTimeout is the default timeout duration of async put container operation.
	AsyncPutContainerOperationTimeout = time.Duration(5) * time.Minute
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

	old := &datamodel.ContainerResource{}

	isNewResource := false
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), old)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			isNewResource = true
		} else {
			return nil, err
		}
	}
	if req.Method == http.MethodPatch && isNewResource {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	if !isNewResource && !old.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	newResource.SystemData = ctrl.UpdateSystemData(old.SystemData, *serviceCtx.SystemData())
	if !isNewResource {
		newResource.CreatedAPIVersion = old.CreatedAPIVersion
		prop := newResource.Properties.BasicResourceProperties
		if !old.Properties.BasicResourceProperties.EqualLinkedResource(prop) {
			return rest.NewLinkedResourceUpdateErrorResponse(serviceCtx.ResourceID.String(), &old.Properties.BasicResourceProperties), nil
		}
		// Container is a resource that is asyncly processed. Here, in createOrUpdateContainer, we
		// don't do the rendering and the deployment. newResource is collected from the request and
		// that is why newResource doesn't have outputResources. It is wiped in the save call 2
		// lines below. Because we are saving newResource and newResource doesn't have the output
		// resources array. When we don't know the outputResources of a resource, we can't delete
		// the ones that are not needed when we are deploying a new version of that resource.
		// Container X - v1 => OutputResources[Y,Z]
		// During the createOrUpdateContainer call Container X loses the OutputResources array
		// because it is wiped from the DB when we are saving the newResource.
		// Container X - v2 needs to be deployed and because we don't know the outputResources
		// of v1, we don't know which one to delete.
		newResource.Properties.BasicResourceProperties = old.Properties.BasicResourceProperties
	}

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
	dm.Properties.ProvisioningState = v1.ProvisioningStateAccepted
	dm.TenantID = serviceCtx.HomeTenantID
	dm.CreatedAPIVersion = dm.UpdatedAPIVersion
	return dm, err
}
