// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

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

var _ armrpc_controller.Controller = (*GetPlane)(nil)

// GetPlane is the controller implementation to get the details of a UCP Plane.
type GetPlane struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewDeletePlane creates a new DeletePlane.
func NewGetPlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetPlane{
		armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		),
	}, nil
}

func (p *GetPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger.Info(fmt.Sprintf("Getting plane %s from db", serviceCtx.ResourceID))
	plane, etag, err := p.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if plane == nil {
		return armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return p.ConstructSyncResponse(ctx, req.Method, etag, plane)
}
