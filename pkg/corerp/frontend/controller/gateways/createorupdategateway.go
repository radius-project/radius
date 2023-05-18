// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package gateways

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

var _ ctrl.Controller = (*CreateOrUpdateGateway)(nil)

// AsyncPutGatewayOperationTimeout is the default timeout duration of async put gateway operation.
var AsyncPutGatewayOperationTimeout = time.Duration(120) * time.Second

// CreateOrUpdateGateway is the controller implementation to create or update a gateway resource.
type CreateOrUpdateGateway struct {
	ctrl.Operation[*datamodel.Gateway, datamodel.Gateway]
}

// NewCreateOrUpdateGateway creates a new CreateOrUpdateGateway.
func NewCreateOrUpdateGateway(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateGateway{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Gateway]{
				RequestConverter:  converter.GatewayDataModelFromVersioned,
				ResponseConverter: converter.GatewayDataModelToVersioned,
			},
		),
	}, nil
}

// Run executes CreateOrUpdateGateway operation.
func (e *CreateOrUpdateGateway) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
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

	if r, err := rp_frontend.PrepareRadiusResource(ctx, newResource, old, e.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := ValidateAndMutateRequest(ctx, newResource, old, e.Options()); r != nil || err != nil {
		return r, err
	}

	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, AsyncPutGatewayOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
