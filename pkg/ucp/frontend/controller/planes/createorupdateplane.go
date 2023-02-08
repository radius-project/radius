// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	http "net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
)

var _ armrpc_controller.Controller = (*CreateOrUpdatePlane)(nil)

// CreateOrUpdatePlane is the controller implementation to create/update a UCP plane resource.
type CreateOrUpdatePlane struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewCreateOrUpdatePlane creates a new CreateOrUpdatePlane.
func NewCreateOrUpdatePlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdatePlane{
		armrpc_controller.NewOperation(opts.CommonControllerOptions,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		),
	}, nil
}

func (p *CreateOrUpdatePlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := p.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := p.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := p.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := p.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return p.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
