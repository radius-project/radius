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
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*GetPlane)(nil)

// GetPlane is the controller implementation to get a UCP plane.
type GetPlane struct {
	ctrl.Operation[*datamodel.Plane, datamodel.Plane]
}

// GetPlane gets a UCP plane.
func NewGetPlane(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetPlane{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Plane]{},
		),
	}, nil
}

func (p *GetPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.Options().BasePath, req.URL.Path)
	logger := logr.FromContextOrDiscard(ctx)
	_, err := resources.ParseScope(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger.Info(fmt.Sprintf("Getting plane %s from db", serviceCtx.ResourceID))
	plane, _, err := p.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if plane == nil {
		restResponse := armrpc_rest.NewNotFoundResponse(serviceCtx.ResourceID)
		return restResponse, nil
	}

	sCtx := v1.ARMRequestContextFromContext(ctx)

	switch sCtx.APIVersion {
	case v20220901privatepreview.Version:
		versioned, err := converter.PlaneDataModelToVersioned(plane, serviceCtx.APIVersion)
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
