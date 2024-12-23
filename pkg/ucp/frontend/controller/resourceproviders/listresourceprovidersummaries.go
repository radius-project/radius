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
package resourceproviders

import (
	"context"
	"errors"
	http "net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ armrpc_controller.Controller = (*ListResourceProviderSummaries)(nil)

// ListResourceProviderSummaries is the controller implementation to list the summaries
// of all resource providers.
type ListResourceProviderSummaries struct {
	armrpc_controller.Operation[*datamodel.ResourceProviderSummary, datamodel.ResourceProviderSummary]
}

// NewListResourceProviderSummaries creates a new controller for listing the summaries of all resource providers.
func NewListResourceProviderSummaries(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ListResourceProviderSummaries{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.ResourceProviderSummary]{
				RequestConverter:  converter.ResourceProviderSummaryDataModelFromVersioned,
				ResponseConverter: converter.ResourceProviderSummaryDataModelToVersioned,
			},
		),
	}, nil
}

// Run implements controller.Controller.
func (r *ListResourceProviderSummaries) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	relativePath := middleware.GetRelativePath(r.Options().PathBase, req.URL.Path)

	// NOTE: the URL path should be something like: /planes/radius/local/providers.
	//
	// This is NOT a valid resource id, so we can't use the parser for it.
	//
	// Instead we trim this to just /planes/radius/local
	scope, err := resources.ParseScope(
		strings.TrimSuffix(
			strings.TrimSuffix(relativePath, resources.SegmentSeparator), resources.SegmentSeparator+resources.ProvidersSegment))
	if err != nil {
		return nil, err
	}

	// First check if the plane exists.
	_, err = r.DatabaseClient().Get(ctx, scope.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		return armrpc_rest.NewNotFoundResponse(scope), nil
	} else if err != nil {
		return nil, err
	}

	// Now query for resource provider summaries
	query := database.Query{
		RootScope:    scope.String(),
		ResourceType: datamodel.ResourceProviderSummaryResourceType,
	}

	result, err := r.DatabaseClient().Query(ctx, query)
	if err != nil {
		return nil, err
	}

	response, err := r.createResponse(ctx, result)
	if err != nil {
		return nil, err
	}

	return armrpc_rest.NewOKResponse(response), nil
}

func (r *ListResourceProviderSummaries) createResponse(ctx context.Context, result *database.ObjectQueryResult) (*v1.PaginatedList, error) {
	items := v1.PaginatedList{
		Value: []any{}, // Initialize to empty list for testability
	}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	for _, item := range result.Items {
		data := datamodel.ResourceProviderSummary{}
		err := item.As(&data)
		if err != nil {
			return nil, err
		}

		versioned, err := converter.ResourceProviderSummaryDataModelToVersioned(&data, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}

		items.Value = append(items.Value, versioned)
	}

	return &items, nil
}
