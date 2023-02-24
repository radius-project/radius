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
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListPlanes)(nil)

// ListPlanes is the controller implementation to get the list of UCP planes.
type ListPlanes struct {
	ctrl.Operation[*datamodel.Plane, datamodel.Plane]
}

// GetPlane gets a UCP plane.
func NewListPlanes(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListPlanes{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.Plane]{},
		),
	}, nil
}

func (p *ListPlanes) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	fmt.Println(serviceCtx.APIVersion)
	path := middleware.GetRelativePath(p.Options().BasePath, req.URL.Path)
	logger := logr.FromContextOrDiscard(ctx)
	query := store.Query{
		RootScope:    path,
		IsScopeQuery: true,
	}
	logger.Info(fmt.Sprintf("Listing planes in scope %s", query.RootScope))
	result, err := p.StorageClient().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	listOfPlanes, err := p.createResponse(ctx, req, result)
	if err != nil {
		return nil, err
	}
	var ok = armrpc_rest.NewOKResponse(&v1.PaginatedList{
		Value: listOfPlanes,
	})
	return ok, nil
}

func (p *ListPlanes) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) ([]any, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	listOfPlanes := []any{}
	if len(result.Items) > 0 {
		for _, item := range result.Items {
			var plane datamodel.Plane
			err := item.As(&plane)
			if err != nil {
				return nil, err
			}

			versioned, err := converter.PlaneDataModelToVersioned(&plane, serviceCtx.APIVersion)
			if err != nil {
				return nil, err
			}

			listOfPlanes = append(listOfPlanes, versioned)
		}
	}
	return listOfPlanes, nil
}
