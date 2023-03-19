// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"errors"
	"fmt"
	http "net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*DeleteResourceGroup)(nil)

// DeleteResourceGroup is the controller implementation to delete a UCP resource group.
type DeleteResourceGroup struct {
	armrpc_controller.Operation[*datamodel.ResourceGroup, datamodel.ResourceGroup]
}

// NewDeleteResourceGroup creates a new DeleteResourceGroup.
func NewDeleteResourceGroup(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &DeleteResourceGroup{
		Operation: armrpc_controller.NewOperation(opts.Options,
			armrpc_controller.ResourceOptions[datamodel.ResourceGroup]{
				RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
				ResponseConverter: converter.ResourceGroupDataModelToVersioned,
			},
		),
	}, nil
}

func (r *DeleteResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	logger := ucplog.FromContextOrDiscard(ctx)

	old, etag, err := r.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	// Get all resources under the path with resource group prefix
	listOfResources, err := r.listResources(ctx, r.Options().StorageClient, serviceCtx.ResourceID.String())
	if err != nil {
		return nil, err
	}

	if len(listOfResources.Value) != 0 {
		var resources string
		for _, r := range listOfResources.Value {
			resources += r.ID + "\n"
		}
		logger.Info(fmt.Sprintf("Found %d resources in resource group %s:\n%s", len(listOfResources.Value), serviceCtx.ResourceID, resources))
		return armrpc_rest.NewConflictResponse("Resource group is not empty and cannot be deleted"), nil
	}

	if r, err := r.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if err := r.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}
	logger.Info(fmt.Sprintf("Delete resource group %s successfully", serviceCtx.ResourceID))
	return rest.NewOKResponse(nil), nil
}

func (e *DeleteResourceGroup) listResources(ctx context.Context, db store.StorageClient, path string) (datamodel.ResourceList, error) {
	var query store.Query
	query.RootScope = path
	query.ScopeRecursive = true
	query.IsScopeQuery = false

	result, err := e.StorageClient().Query(ctx, query)
	if err != nil {
		return datamodel.ResourceList{}, err
	}

	if result == nil || len(result.Items) == 0 {
		return datamodel.ResourceList{}, nil
	}

	listOfResources := datamodel.ResourceList{}
	for _, item := range result.Items {
		var resource datamodel.Resource
		err = item.As(&resource)
		if err != nil {
			return listOfResources, err
		}
		listOfResources.Value = append(listOfResources.Value, resource)
	}

	return listOfResources, nil
}
