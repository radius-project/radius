// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package containers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var _ ctrl.Controller = (*CreateOrUpdateContainer)(nil)

var (
	// AsyncPutContainerOperationTimeout is the default timeout duration of async put container operation.
	AsyncPutContainerOperationTimeout = time.Duration(5) * time.Minute
)

// CreateOrUpdateContainer is the controller implementation to create or update a container resource.
type CreateOrUpdateContainer struct {
	ctrl.Operation[*datamodel.ContainerResource, datamodel.ContainerResource]
}

// NewCreateOrUpdateContainer creates a new CreateOrUpdateContainer.
func NewCreateOrUpdateContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateContainer{
		ctrl.NewOperation(opts, converter.ContainerDataModelFromVersioned, converter.ContainerDataModelToVersioned),
	}, nil
}

// Run executes CreateOrUpdateContainer operation.
func (e *CreateOrUpdateContainer) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, isNewResource, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if err := e.ValidateResource(ctx, req, newResource, old, etag, isNewResource); err != nil {
		return nil, err
	}

	if isNewResource {
		newResource.UpdateMetadata(serviceCtx, nil)
	} else {
		newResource.UpdateMetadata(serviceCtx, &old.SystemData)

		if !old.Properties.ProvisioningState.IsTerminal() {
			return rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState)), nil
		}

		if err := e.ValidateLinkedResource(serviceCtx.ResourceID, isNewResource, &newResource.Properties.BasicResourceProperties, &old.Properties.BasicResourceProperties); err != nil {
			return nil, err
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
		newResource.Properties.Status.DeepCopy(&old.Properties.Status)
	}

	newResource.Properties.ProvisioningState = v1.ProvisioningStateAccepted

	obj, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	if err := e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncPutContainerOperationTimeout); err != nil {
		newResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, obj.ETag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
