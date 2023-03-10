// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

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

var _ armrpc_controller.Controller = (*CreateOrUpdateResourceGroup)(nil)

// CreateOrUpdateResourceGroup is the controller implementation to create/update a UCP resource group.
type CreateOrUpdateResourceGroup struct {
	armrpc_controller.Operation[*datamodel.ResourceGroup, datamodel.ResourceGroup]
}

// NewCreateOrUpdateResourceGroup creates a new CreateOrUpdateResourceGroup.
func NewCreateOrUpdateResourceGroup(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateResourceGroup{
		armrpc_controller.NewOperation(opts.CommonControllerOptions,
			armrpc_controller.ResourceOptions[datamodel.ResourceGroup]{
				RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
				ResponseConverter: converter.ResourceGroupDataModelToVersioned,
			},
		),
	}, nil
}

func (r *CreateOrUpdateResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := r.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	old, etag, err := r.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := r.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	newResource.SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := r.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return r.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
