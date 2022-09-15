// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"context"
	"fmt"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var (
	_ ctrl.Controller = (*DeleteHTTPRoute)(nil)
	// AsyncDeleteHTTPRouteOperationTimeout is the default timeout duration of async delete httproute operation.
	AsyncDeleteHTTPRouteOperationTimeout = time.Duration(120) * time.Second
)

// DeleteHTTPRoute is the controller implementation to delete HTTPRoute resource.
type DeleteHTTPRoute struct {
	ctrl.Operation[*rm, rm]
}

// NewDeleteHTTPRoute creates a new DeleteHTTPRoute.
func NewDeleteHTTPRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteHTTPRoute{
		ctrl.NewOperation(opts, converter.HTTPRouteDataModelFromVersioned, converter.HTTPRouteDataModelToVersioned),
	}, nil
}

// Run executes DeleteHTTPRoute operation
func (e *DeleteHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if err := e.ValidateResource(ctx, req, nil, old, etag); err != nil {
		return nil, err
	}

	if !old.Properties.ProvisioningState.IsTerminal() {
		return nil, rest.NewConflictResponse(fmt.Sprintf(ctrl.InProgressStateMessageFormat, old.Properties.ProvisioningState))
	}

	if err := e.StatusManager().QueueAsyncOperation(ctx, serviceCtx, AsyncDeleteHTTPRouteOperationTimeout); err != nil {
		old.Properties.ProvisioningState = v1.ProvisioningStateFailed
		_, rbErr := e.SaveResource(ctx, serviceCtx.ResourceID.String(), old, etag)
		if rbErr != nil {
			return nil, rbErr
		}
		return nil, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
