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
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteContainer)(nil)

var (
	// AsyncDeleteContainerOperationTimeout is the default timeout duration of async delete container operation.
	AsyncDeleteContainerOperationTimeout = time.Duration(120) * time.Second
)

// DeleteContainer is the controller implementation to delete container resource.
type DeleteContainer struct {
	ctrl.BaseController
}

// NewDeleteContainer creates a new DeleteContainer.
func NewDeleteContainer(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteContainer{ctrl.NewBaseController(opts)}, nil
}

func (dc *DeleteContainer) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingContainer := &datamodel.ContainerResource{}
	etag, err := dc.GetResource(ctx, serviceCtx.ResourceID.String(), existingContainer)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNoContentResponse(), nil
	}

	if !existingContainer.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(controller.OngoingAsyncOperationOnResourceMessage), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	err = dc.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncDeleteContainerOperationTimeout)
	if err != nil {
		existingContainer.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := dc.SaveResource(ctx, serviceCtx.ResourceID.String(), existingContainer, etag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	existingContainer.Properties.ProvisioningState = v1.ProvisioningStateDeleting

	return rest.NewAsyncOperationResponse(existingContainer, existingContainer.TrackedResource.Location, http.StatusAccepted,
		serviceCtx.ResourceID, serviceCtx.OperationID, serviceCtx.APIVersion), nil
}
