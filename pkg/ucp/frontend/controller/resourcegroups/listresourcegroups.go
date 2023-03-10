// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

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
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*ListResourceGroups)(nil)

// ListResourceGroups is the controller implementation to get the list of UCP resource groups.
type ListResourceGroups struct {
	armrpc_controller.Operation[*datamodel.ResourceGroup, datamodel.ResourceGroup]
	basePath string
}

// NewListResourceGroups creates a new ListResourceGroups.
func NewListResourceGroups(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &ListResourceGroups{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.ResourceGroup]{
				RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
				ResponseConverter: converter.ResourceGroupDataModelToVersioned,
			},
		),
		basePath: opts.BasePath,
	}, nil
}

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
	apiVersion := ctrl.GetAPIVersion(req)

	for _, item := range result.Items {
		var rg datamodel.ResourceGroup
		err := item.As(&rg)
		if err != nil {
			return nil, err
		}

		versioned, err := converter.ResourceGroupDataModelToVersioned(&rg, apiVersion)
		if err != nil {
			return nil, err
		}

		items.Value = append(items.Value, versioned)
	}

	return &items, nil
}
