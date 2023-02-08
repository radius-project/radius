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
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*GetPlane)(nil)

// GetPlane is the controller implementation to get the details of a UCP Plane.
type GetPlane struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
	basePath string
}

// NewGetPlane creates a new GetPlane.
func NewGetPlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetPlane{
		Operation: armrpc_controller.NewOperation(opts.CommonControllerOptions,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		),
		basePath: opts.BasePath,
	}, nil
}

func (p *GetPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.basePath, req.URL.Path)
	logger := ucplog.FromContextOrDiscard(ctx)
	_, err := resources.ParseScope(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger.Info(fmt.Sprintf("Getting plane %s from db", serviceCtx.ResourceID))
	plane, etag, err := p.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if plane == nil {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return p.ConstructSyncResponse(ctx, req.Method, etag, plane)
}
