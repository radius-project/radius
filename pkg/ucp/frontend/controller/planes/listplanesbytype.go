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
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListPlanesByType)(nil)

// ListPlanesByType is the controller implementation to get the list of UCP planes.
type ListPlanesByType struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
}

// # Function Explanation
//
// NewListPlanesByType creates a new controller for listing planes by type and returns it, or an error if the controller
// cannot be created.
func NewListPlanesByType(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ListPlanesByType{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		),
	}, nil
}

// # Function Explanation
//
// ListPlanesByType takes in a request object and returns a list of planes of a given type from the storage client. If
// an error occurs, it returns an error.
func (e *ListPlanesByType) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(e.Options().PathBase, req.URL.Path)
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
