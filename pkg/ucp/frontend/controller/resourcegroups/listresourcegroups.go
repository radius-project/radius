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
	"fmt"
	http "net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListResourceGroups)(nil)

// ListResourceGroups is the controller implementation to get the list of UCP resource groups.
type ListResourceGroups struct {
	armrpc_controller.Operation[*datamodel.ResourceGroup, datamodel.ResourceGroup]
}

// NewListResourceGroups creates a new controller for listing resource groups.
func NewListResourceGroups(opts armrpc_controller.Options) (armrpc_controller.Controller, error) {
	return &ListResourceGroups{
		Operation: armrpc_controller.NewOperation(opts,
			armrpc_controller.ResourceOptions[datamodel.ResourceGroup]{
				RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
				ResponseConverter: converter.ResourceGroupDataModelToVersioned,
			},
		),
	}, nil
}

// Run() function extracts the plane type and name from the request URL, queries the storage client for resource groups in the
// scope of the plane, creates a response with the list of resource groups and returns an OK response with the list of resource groups.
func (r *ListResourceGroups) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	planeType, planeName, _, err := resources.ExtractPlanesPrefixFromURLPath(serviceCtx.ResourceID.String())
	if err != nil {
		return nil, err
	}

	var query store.Query
	query.RootScope = resources.SegmentSeparator + resources.PlanesSegment + resources.SegmentSeparator + planeType + resources.SegmentSeparator + planeName
	query.IsScopeQuery = true
	query.ResourceType = "resourcegroups"
	logger.Info(fmt.Sprintf("Listing resource groups in scope %s", query.RootScope))

	result, err := r.StorageClient().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	listOfResourceGroups, err := r.createResponse(ctx, req, result)
	if err != nil {
		return nil, err
	}

	var ok = armrpc_rest.NewOKResponse(listOfResourceGroups)
	return ok, nil
}

func (e *ListResourceGroups) createResponse(ctx context.Context, req *http.Request, result *store.ObjectQueryResult) (*v1.PaginatedList, error) {
	items := v1.PaginatedList{}
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	for _, item := range result.Items {
		var rg datamodel.ResourceGroup
		err := item.As(&rg)
		if err != nil {
			return nil, err
		}

		versioned, err := converter.ResourceGroupDataModelToVersioned(&rg, serviceCtx.APIVersion)
		if err != nil {
			return nil, err
		}

		items.Value = append(items.Value, versioned)
	}

	return &items, nil
}
