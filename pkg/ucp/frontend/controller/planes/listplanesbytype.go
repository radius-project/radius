/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package planes

import (
	"context"
	"fmt"
	http "net/http"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListPlanesByType)(nil)

// ListPlanesByType is the controller implementation to get the list of UCP planes.
type ListPlanesByType struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
	basePath string
}

// NewListPlanesByType creates a new ListPlanesByType.
//
// # Function Explanation
// 
//	ListPlanesByType creates a new controller with the given options and returns it, or an error if something goes wrong. It
//	 uses the armrpc_controller package to create a new operation with the given options, and sets the basePath field of the
//	 controller. If an error occurs, it is returned to the caller.
func NewListPlanesByType(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ListPlanesByType{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		),
		basePath: opts.BasePath,
	}, nil
}

// # Function Explanation
// 
//	The ListPlanesByType function queries the storage client for planes of a given type and returns a list of planes in an 
//	OKResponse. If an error occurs, it is returned to the caller.
func (e *ListPlanesByType) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(e.basePath, req.URL.Path)
	// The path is /planes/{planeType}
	planeType := strings.Split(path, resources.SegmentSeparator)[2]
	query := store.Query{
		RootScope:    resources.SegmentSeparator + resources.PlanesSegment,
		IsScopeQuery: true,
		ResourceType: planeType,
	}
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Listing planes in scope %s/%s", query.RootScope, planeType))
	result, err := e.StorageClient().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	listOfPlanes, err := e.createResponse(ctx, req, result)
	if err != nil {
		return nil, err
	}
	var ok = armrpc_rest.NewOKResponse(listOfPlanes)
	return ok, nil
}

func (p *ListPlanesByType) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	items := v1.PaginatedList{}

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

		items.Value = append(items.Value, versioned)
	}

	return &items, nil
}
