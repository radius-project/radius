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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListPlanes)(nil)

// ListPlanes is the controller implementation to get the list of UCP planes.
type ListPlanes struct {
	armrpc_controller.Operation[*datamodel.Plane, datamodel.Plane]
}

// NewListPlanes creates a new ListPlanes.
//
// # Function Explanation
// 
//	NewListPlanes creates a new ListPlanes controller which handles requests for the Plane resource type, converting the 
//	request and response data to and from the versioned data model. It returns an error if the controller cannot be created.
func NewListPlanes(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ListPlanes{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		),
	}, nil
}

// # Function Explanation
// 
//	ListPlanes runs a query on the storage client to list all planes in the given scope, creates a response from the query 
//	result, and returns an OKResponse with the list of planes. If an error occurs during the query or response creation, an 
//	error is returned.
func (e *ListPlanes) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger := ucplog.FromContextOrDiscard(ctx)

	query := store.Query{
		RootScope:    serviceCtx.ResourceID.String(),
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
	var ok = armrpc_rest.NewOKResponse(listOfPlanes)
	return ok, nil
}

func (p *ListPlanes) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
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
