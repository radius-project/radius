// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"fmt"
	http "net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*GetResourceGroup)(nil)

// GetResourceGroup is the controller implementation to get the details of a UCP resource group
type GetResourceGroup struct {
	armrpc_controller.Operation[*datamodel.ResourceGroup, datamodel.ResourceGroup]
	basePath string
}

// NewGetResourceGroup creates a new GetResourceGroup.
func NewGetResourceGroup(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetResourceGroup{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.ResourceGroup]{
				RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
				ResponseConverter: converter.ResourceGroupDataModelToVersioned,
			},
		),
		basePath: opts.BasePath,
	}, nil
}

func (r *GetResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger.Info(fmt.Sprintf("Getting resource group %s from db", serviceCtx.ResourceID))

	rg, etag, err := r.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if rg == nil {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return r.ConstructSyncResponse(ctx, req.Method, etag, rg)
}
