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
	"fmt"
	http "net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ armrpc_controller.Controller = (*GetResourceProviderSummary)(nil)

// GetResourceProviderSummary is the controller implementation to get the list of resources stored in a resource group.
type GetResourceProviderSummary struct {
	armrpc_controller.Operation[*datamodel.ResourceProviderSummary, datamodel.ResourceProviderSummary]
}

// NewGetResourceProviderSummary creates a new controller for listing resources stored in a resource group.
func NewGetResourceProviderSummary(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &GetResourceProviderSummary{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.ResourceProviderSummary]{
				RequestConverter:  converter.ResourceProviderSummaryDataModelFromVersioned,
				ResponseConverter: converter.ResourceProviderSummaryDataModelToVersioned,
			},
		),
	}, nil
}

// Run implements controller.Controller.
func (r *GetResourceProviderSummary) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	relativePath := middleware.GetRelativePath(r.Options().PathBase, req.URL.Path)

	scope, name, err := r.extractScopeAndName(relativePath)
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

	// Next, construct a resource id for the summary.
	id, err := datamodel.ResourceProviderSummaryIDFromParts(scope.String(), name)
	if err != nil {
		return nil, err
	}

	result, err := r.DatabaseClient().Get(ctx, id.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		// If we fail to find the summary, then use the relative path as the target.
		message := fmt.Sprintf("the resource provider with name '%s' was not found", name)
		return armrpc_rest.NewNotFoundMessageResponse(message), nil
	} else if err != nil {
		return nil, err
	}

	response, err := r.createResponse(ctx, result)
	if err != nil {
		return nil, err
	}

	return armrpc_rest.NewOKResponse(response), nil
}

func (r *GetResourceProviderSummary) extractScopeAndName(relativePath string) (resources.ID, string, error) {
	// Trim a trailing slash if it exists.
	relativePath = strings.TrimSuffix(relativePath, "/")

	// NOTE: the URL path should be something like: /planes/radius/local/providers/Applications.Test.
	//
	// This is NOT a valid resource id, so we can't use the parser for it.
	//
	// Instead we trim this to just /planes/radius/local and keep the Applications.Test part separate.
	lastSeparator := strings.LastIndex(relativePath, resources.SegmentSeparator)
	if lastSeparator == -1 {
		// This probably can't happen, but let's not panic.
		return resources.ID{}, "", errors.New("invalid URL path")
	}

	name := relativePath[lastSeparator+1:]
	scope, err := resources.ParseScope(
		strings.TrimSuffix(
			strings.TrimSuffix(relativePath[0:lastSeparator], resources.SegmentSeparator), resources.SegmentSeparator+resources.ProvidersSegment))
	if err != nil {
		return resources.ID{}, "", err
	}

	return scope, name, nil
}

func (r *GetResourceProviderSummary) createResponse(ctx context.Context, result *database.Object) (any, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	summary := datamodel.ResourceProviderSummary{}
	err := result.As(&summary)
	if err != nil {
		return nil, err
	}

	return converter.ResourceProviderSummaryDataModelToVersioned(&summary, serviceCtx.APIVersion)
}
