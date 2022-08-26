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

	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*GetPlane)(nil)

// GetPlane is the controller implementation to get the details of a UCP Plane.
type GetPlane struct {
	ctrl.BaseController
}

// NewGetPlane creates a new GetPlane.
func NewGetPlane(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetPlane{ctrl.NewBaseController(opts)}, nil
}

func (p *GetPlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)
	logger := ucplog.GetLogger(ctx)
	resourceId, err := resources.Parse(path)
	if err != nil {
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	logger.Info(fmt.Sprintf("Getting plane %s from db", resourceId))
	plane := datamodel.Plane{}
	_, err = p.GetResource(ctx, resourceId.String(), &plane)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNotFoundResponse(path)
			logger.Info(fmt.Sprintf("Plane %s not found in db", resourceId))
			return restResponse, nil
		}
		return nil, err
	}

	apiVersion := ctrl.GetAPIVersion(logger, req)
	versioned, _ := converter.PlaneDataModelToVersioned(&plane, apiVersion)
	return rest.NewOKResponse(versioned), nil
}
