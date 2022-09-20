// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
)

var (
	_ ctrl.Controller = (*DeleteHTTPRoute)(nil)
	// AsyncDeleteHTTPRouteOperationTimeout is the default timeout duration of async delete httproute operation.
	AsyncDeleteHTTPRouteOperationTimeout = time.Duration(120) * time.Second
)

// DeleteHTTPRoute is the controller implementation to delete HTTPRoute resource.
type DeleteHTTPRoute struct {
	ctrl.Operation[*datamodel.HTTPRoute, datamodel.HTTPRoute]
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

	if r, err := e.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := e.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, AsyncDeleteHTTPRouteOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
