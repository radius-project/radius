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
package resourcegroups

import (
	"context"
	"errors"
	http "net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
)

var _ armrpc_controller.Controller = (*ListResources)(nil)

// ListResources is the controller implementation to get the list of resources stored in a resource group.
type ListResources struct {
	armrpc_controller.Operation[*datamodel.GenericResource, datamodel.GenericResource]
}

// NewListResources creates a new controller for listing resources stored in a resource group.
func NewListResources(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ListResources{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.GenericResource]{
				RequestConverter:  converter.GenericResourceDataModelFromVersioned,
				ResponseConverter: converter.GenericResourceDataModelToVersioned,
			},
		),
	}, nil
}

// Run implements controller.Controller.
func (r *ListResources) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	relativePath := middleware.GetRelativePath(r.Options().PathBase, req.URL.Path)
	id, err := resources.Parse(relativePath)
	if err != nil {
		return nil, err
	}

	// Cut off the "resources" part of the ID. The ID should be the ID of a resource group.
	resourceGroupID := id.Truncate()

	// First check if the resource group exists.
	_, err = r.StorageClient().Get(ctx, resourceGroupID.String())
	if errors.Is(err, &store.ErrNotFound{}) {
		return armrpc_rest.NewNotFoundResponse(id), nil
	} else if err != nil {
		return nil, err
	}

	query := store.Query{
		RootScope:    resourceGroupID.String(),
		ResourceType: v20231001preview.ResourceType,
	}

	result, err := r.StorageClient().Query(ctx, query)
	if err != nil {
		return nil, err
	}

	response, err := r.createResponse(ctx, req, result)
	if err != nil {
		return nil, err
	}

	return armrpc_rest.NewOKResponse(response), nil
}

func (r *ListResources) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	items := v1.PaginatedList{}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	for _, item := range result.Items {
		data := datamodel.GenericResource{}
		err := item.As(&data)
		if err != nil {
			return nil, err
		}

		versioned, err := converter.GenericResourceDataModelToVersioned(&data, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}

		items.Value = append(items.Value, versioned)
	}

	return &items, nil
}
