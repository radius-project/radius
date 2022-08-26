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
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*DeletePlane)(nil)

// DeletePlane is the controller implementation to delete a UCP Plane.
type DeletePlane struct {
	ctrl.BaseController
}

// NewDeletePlane creates a new DeletePlane.
func NewDeletePlane(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeletePlane{ctrl.NewBaseController(opts)}, nil
}

func (p *DeletePlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(p.Options.BasePath, req.URL.Path)
	logger := ucplog.GetLogger(ctx)
	resourceId, err := resources.Parse(path)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}
	existingPlane := datamodel.Plane{}
	etag, err := p.GetResource(ctx, resourceId.String(), &existingPlane)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := rest.NewNoContentResponse()
			return restResponse, nil
		}
		return nil, err
	}

	err = p.DeleteResource(ctx, resourceId.String(), etag)
	if err != nil {
		return nil, err
	}
	restResponse := rest.NewNoContentResponse()
	logger.Info(fmt.Sprintf("Successfully deleted plane %s", resourceId))
	return restResponse, nil
}
