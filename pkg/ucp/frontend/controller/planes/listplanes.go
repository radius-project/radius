// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package planes

import (
	"context"
	"fmt"
	http "net/http"

	"github.com/project-radius/radius/pkg/middleware"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*ListPlanes)(nil)

// ListPlanes is the controller implementation to get the list of UCP planes.
type ListPlanes struct {
	ctrl.BaseController
}

// NewListPlanes creates a new ListPlanes.
func NewListPlanes(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListPlanes{ctrl.NewBaseController(opts)}, nil
}

func (e *ListPlanes) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(e.Options.BasePath, req.URL.Path)
	logger := ucplog.GetLogger(ctx)

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
	var ok = rest.NewOKResponse(listOfPlanes)
	return ok, nil
}

func (p *ListPlanes) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (rest.PlaneList, error) {
	listOfPlanes := rest.PlaneList{}
	if len(result.Items) > 0 {
		for _, item := range result.Items {
			var plane rest.Plane
			err := item.As(&plane)
			if err != nil {
				return rest.PlaneList{}, err
			}
			listOfPlanes.Value = append(listOfPlanes.Value, plane)
		}
	}
	return listOfPlanes, nil
}
