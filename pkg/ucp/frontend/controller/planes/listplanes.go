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
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListPlanes)(nil)

// ListPlanes is the controller implementation to get the list of UCP planes.
type ListPlanes struct {
	ctrl.BaseController
}

// NewListPlanes creates a new ListPlanes.
func NewListPlanes(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ListPlanes{ctrl.NewBaseController(opts)}, nil
}

func (e *ListPlanes) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(e.Options.BasePath, req.URL.Path)
	logger := ucplog.FromContextOrDiscard(ctx)

	query := store.Query{
		RootScope:    path,
		IsScopeQuery: true,
	}
	logger.Info(fmt.Sprintf("Listing planes in scope %s", query.RootScope))
	result, err := e.StorageClient().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	listOfPlanes, err := e.createResponse(ctx, req, result)
	if err != nil {
		return nil, err
	}
	var ok = armrpc_rest.NewOKResponse(&v1.PaginatedList{
		Value: listOfPlanes,
	})
	return ok, nil
}

func (p *ListPlanes) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) ([]any, error) {
	apiVersion := ctrl.GetAPIVersion(req)
	listOfPlanes := []any{}
	if len(result.Items) > 0 {
		for _, item := range result.Items {
			var plane datamodel.Plane
			err := item.As(&plane)
			if err != nil {
				return nil, err
			}

			versioned, err := converter.PlaneDataModelToVersioned(&plane, apiVersion)
			if err != nil {
				return nil, err
			}

			listOfPlanes = append(listOfPlanes, versioned)
		}
	}
	return listOfPlanes, nil
}
