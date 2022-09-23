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
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
)

var (
	_ ctrl.Controller = (*CreateOrUpdateHTTPRoute)(nil)

	// AsyncPutHTTPRouteOperationTimeout is the default timeout duration of async put httproute operation.
	AsyncPutHTTPRouteOperationTimeout = time.Duration(120) * time.Second
)

// CreateOrUpdateHTTPRoute is the controller implementation to create or update HTTPRoute resource.
type CreateOrUpdateHTTPRoute struct {
	ctrl.Operation[*datamodel.HTTPRoute, datamodel.HTTPRoute]
}

// NewCreateOrUpdateTTPRoute creates a new CreateOrUpdateHTTPRoute.
func NewCreateOrUpdateHTTPRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateHTTPRoute{
		ctrl.NewOperation(opts, converter.HTTPRouteDataModelFromVersioned, converter.HTTPRouteDataModelToVersioned),
	}, nil
}

// Run executes CreateOrUpdateHTTPRoute operation.
func (e *CreateOrUpdateHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := e.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := rp_frontend.PrepareRadiusResource(ctx, old, newResource); r != nil || err != nil {
		return r, err
	}

	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, AsyncPutHTTPRouteOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
