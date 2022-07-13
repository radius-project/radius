// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

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

var (
	_ ctrl.Controller = (*DeleteHTTPRoute)(nil)
	// AsyncDeleteHTTPRouteOperationTimeout is the default timeout duration of async delete httproute operation.
	AsyncDeleteHTTPRouteOperationTimeout = time.Duration(120) * time.Second
)

// DeleteHTTPRoute is the controller implementation to delete HTTPRoute resource.
type DeleteHTTPRoute struct {
	ctrl.BaseController
}

// NewDeleteHTTPRoute creates a new DeleteHTTPRoute.
func NewDeleteHTTPRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteHTTPRoute{ctrl.NewBaseController(opts)}, nil
}

// Run executes DeleteHTTPRoute operation
func (e *DeleteHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.HTTPRoute{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNoContentResponse(), nil
	}

	if !existingResource.Properties.ProvisioningState.IsTerminal() {
		return rest.NewConflictResponse(controller.OngoingAsyncOperationOnResourceMessage), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	err = e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncDeleteHTTPRouteOperationTimeout)
	if err != nil {
		existingResource.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), existingResource, etag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	existingResource.Properties.ProvisioningState = v1.ProvisioningStateDeleting

	return rest.NewAsyncOperationResponse(existingResource, existingResource.TrackedResource.Location, http.StatusAccepted,
		serviceCtx.ResourceID, serviceCtx.OperationID, serviceCtx.APIVersion), nil
}
