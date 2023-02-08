// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"errors"
	http "net/http"

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ armrpc_controller.Controller = (*DeletePlane)(nil)

// DeletePlane is the controller implementation to delete a UCP Plane.
type DeletePlane struct {
	ctrl.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewDeletePlane creates a new DeletePlane.
func NewDeletePlane(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeletePlane{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Plane]{},
		),
	}, nil
}

func (p *DeletePlane) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(p.BasePath(), req.URL.Path)
	resourceId, err := resources.ParseScope(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	old, etag, err := p.GetResource(ctx, resourceId)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := p.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if err := p.StorageClient().Delete(ctx, resourceId.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
