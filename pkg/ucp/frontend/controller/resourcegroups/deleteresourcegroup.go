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

	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*DeleteResourceGroup)(nil)

// DeleteResourceGroup is the controller implementation to delete a UCP resource group.
type DeleteResourceGroup struct {
	ctrl.BaseController
}

// NewDeleteResourceGroup creates a new DeleteResourceGroup.
func NewDeleteResourceGroup(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteResourceGroup{ctrl.NewBaseController(opts)}, nil
}

func (r *DeleteResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(r.Options.BasePath, req.URL.Path)
	logger := ucplog.GetLogger(ctx)
	resourceID, err := resources.ParseScope(path)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	existingRG := rest.ResourceGroup{}
	etag, err := r.GetResource(ctx, resourceID.String(), &existingRG)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			restResponse := armrpc_rest.NewNoContentResponse()
			return restResponse, nil
		}
		return nil, err
	}

	// Get all resources under the path with resource group prefix
	listOfResources, err := r.listResources(ctx, r.Options.DB, path)
	if err != nil {
		return nil, err
	}

	if len(listOfResources.Value) != 0 {
		var resources string
		for _, r := range listOfResources.Value {
			resources += r.ID + "\n"
		}
		logger.Info(fmt.Sprintf("Found %d resources in resource group %s:\n%s", len(listOfResources.Value), resourceID, resources))
		return armrpc_rest.NewConflictResponse("Resource group is not empty and cannot be deleted"), nil
	}

	err = r.DeleteResource(ctx, resourceID.String(), etag)
	if err != nil {
		return nil, err
	}
	restResponse := armrpc_rest.NewNoContentResponse()
	logger.Info(fmt.Sprintf("Delete resource group %s successfully", resourceID))
	return restResponse, nil
}

func (e *DeleteResourceGroup) listResources(ctx context.Context, db store.StorageClient, path string) (rest.ResourceList, error) {
	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldRequestPath, path)
	var query store.Query
	query.RootScope = path
	query.ScopeRecursive = true
	query.IsScopeQuery = false

	result, err := e.StorageClient().Query(ctx, query)
	if err != nil {
		return rest.ResourceList{}, err
	}

	if result == nil || len(result.Items) == 0 {
		return rest.ResourceList{}, nil
	}

	listOfResources := rest.ResourceList{}
	for _, item := range result.Items {
		var resource rest.Resource
		err = item.As(&resource)
		if err != nil {
			return listOfResources, err
		}
		listOfResources.Value = append(listOfResources.Value, resource)
	}

	return listOfResources, nil
}
