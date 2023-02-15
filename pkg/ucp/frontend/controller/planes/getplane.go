// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"fmt"
	http "net/http"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ armrpc_controller.Controller = (*GetPlane)(nil)

// GetPlane is the controller implementation to get the details of a UCP Plane.
type GetPlane struct {
	ctrl.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewGetPlane creates a new GetPlane.
func NewGetPlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetPlane{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Plane]{},
		),
	}, nil
}

func (p *GetPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.BasePath(), req.URL.Path)
	logger := logr.FromContextOrDiscard(ctx)
	resourceId, err := resources.ParseScope(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}
	logger.Info(fmt.Sprintf("Getting plane %s from db", resourceId))
	plane, _, err := p.GetResource(ctx, resourceId)
	if err != nil {
		return nil, err
	}
	if plane == nil {
		restResponse := armrpc_rest.NewNotFoundResponse(resourceId)
		logger.Info(fmt.Sprintf("Plane %s not found in db", resourceId.String()))
		return restResponse, nil
	}

	sCtx := v1.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220901privatepreview.Version:
		apiVersion := ctrl.GetAPIVersion(req)
		versioned, err := converter.PlaneDataModelToVersioned(plane, apiVersion)
		if err != nil {
			return armrpc_rest.NewInternalServerErrorARMResponse(v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: err.Error(),
				},
			}), nil
		}
		return armrpc_rest.NewOKResponse(versioned), nil
	}

	return rest.NewNotFoundAPIVersionResponse("planes", "ucp", sCtx.APIVersion), nil
}
