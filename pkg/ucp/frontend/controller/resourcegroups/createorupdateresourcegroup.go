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

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	armrpc_rest "github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ armrpc_controller.Controller = (*CreateOrUpdateResourceGroup)(nil)

// CreateOrUpdateResourceGroup is the controller implementation to create/update a UCP resource group.
type CreateOrUpdateResourceGroup struct {
	ctrl.BaseController
}

// NewCreateOrUpdateResourceGroup creates a new CreateOrUpdateResourceGroup.
func NewCreateOrUpdateResourceGroup(opts ctrl.Options) (armrpc_controller.Controller, error) {
	return &CreateOrUpdateResourceGroup{ctrl.NewBaseController(opts)}, nil
}

func (r *CreateOrUpdateResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (armrpc_rest.Response, error) {
	path := middleware.GetRelativePath(r.Options.BasePath, req.URL.Path)
	body, err := ctrl.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}

	// Convert to version agnostic data model
	apiVersion := ctrl.GetAPIVersion(req)
	newResource, err := converter.ResourceGroupDataModelFromVersioned(body, apiVersion)
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	id, err := resources.Parse(path)
	//cannot parse ID something wrong with request
	if err != nil {
		return armrpc_rest.NewBadRequestResponse(err.Error()), nil
	}

	// Set TrackedResource properties that come from the URL
	newResource.ID = path
	newResource.Name = id.Name()
	newResource.Type = ResourceGroupType

	logger := ucplog.FromContextWithSpan(ctx)

	existingResource := datamodel.ResourceGroup{}
	rgExists := true
	etag, err := r.GetResource(ctx, id.String(), &existingResource)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			rgExists = false
			logger.Info(fmt.Sprintf("No existing resource group %s found in db", id))
		} else {
			return nil, err
		}
	}

	// Save data model resource group to db
	_, err = r.SaveResource(ctx, id.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	// Return a versioned response of the resource group
	versioned, err := converter.ResourceGroupDataModelToVersioned(newResource, apiVersion)
	if err != nil {
		return nil, err
	}

	restResp := armrpc_rest.NewOKResponse(versioned)
	if rgExists {
		logger.Info(fmt.Sprintf("Updated resource group %s successfully", newResource.TrackedResource.ID))
	} else {
		logger.Info(fmt.Sprintf("Created resource group %s successfully", newResource.TrackedResource.ID))
	}
	return restResp, nil
}
