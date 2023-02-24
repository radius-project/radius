// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"fmt"
	http "net/http"
	"strings"

	"github.com/go-logr/logr"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*GetResourceGroup)(nil)

// GetResourceGroup is the controller implementation to get the details of a UCP resource group
type GetResourceGroup struct {
	ctrl.Operation[*datamodel.ResourceGroup, datamodel.ResourceGroup]
}

// NewGetResourceGroup creates a new GetResourceGroup.
func NewGetResourceGroup(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetResourceGroup{
		ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.ResourceGroup]{
				RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
				ResponseConverter: converter.ResourceGroupDataModelToVersioned,
			},
		),
	}, nil
}

func (r *GetResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	path := middleware.GetRelativePath(r.Options().BasePath, req.URL.Path)
	logger := logr.FromContextOrDiscard(ctx)
	id := strings.ToLower(path)
	resourceID, err := resources.ParseScope(id)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	logger.Info(fmt.Sprintf("Getting resource group %s from db", resourceID))
	// old := &datamodel.ResourceGroup{}

	rg, _, err := r.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if rg == nil {
		logger.Info(fmt.Sprintf("Resource group %s not found in db", resourceID))
		restResponse := armrpc_rest.NewNotFoundResponse(resourceID)
		return restResponse, nil
	}

	// Return a versioned response of the resource group
	versioned, err := converter.ResourceGroupDataModelToVersioned(rg, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	restResponse := armrpc_rest.NewOKResponse(versioned)
	return restResponse, nil
}
