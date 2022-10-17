// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"errors"
	"fmt"
	http "net/http"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*GetPlane)(nil)

// GetPlane is the controller implementation to get the details of a UCP Plane.
type GetPlane struct {
	ctrl.BaseController
}

// NewGetPlane creates a new GetPlane.
func NewGetPlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &GetPlane{ctrl.NewBaseController(opts)}, nil
}

func (p *GetPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)
	logger := ucplog.GetLogger(ctx)
	resourceId, err := resources.ParseScope(path)
	if err != nil {
		if err != nil {
			return armrpc_rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	logger.Info(fmt.Sprintf("Getting plane %s from db", resourceId))
	plane := rest.Plane{}
	_, err = p.GetResource(ctx, resourceId.String(), &plane)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := armrpc_rest.NewNotFoundResponse(resourceId)
			logger.Info(fmt.Sprintf("Plane %s not found in db", resourceId))
			return restResponse, nil
		}
		return nil, err
	}
	restResponse := armrpc_rest.NewOKResponse(plane)
	return restResponse, nil
}
