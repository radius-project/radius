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
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*CreateOrUpdateResourceGroup)(nil)

// CreateOrUpdateResourceGroup is the controller implementation to create/update a UCP resource group.
type CreateOrUpdateResourceGroup struct {
	ctrl.BaseController
}

// NewCreateOrUpdateResourceGroup creates a new CreateOrUpdateResourceGroup.
func NewCreateOrUpdateResourceGroup(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResourceGroup{ctrl.NewBaseController(opts)}, nil
}

func (r *CreateOrUpdateResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(r.Options.BasePath, req.URL.Path)
	body, err := ctrl.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}

	// Convert to version agnostic data model
	logger := ucplog.GetLogger(ctx)
	apiVersion := ctrl.GetAPIVersion(logger, req)
	newResource, err := converter.ResourceGroupDataModelFromVersioned(body, apiVersion)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	id, err := resources.Parse(path)
	//cannot parse ID something wrong with request
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	// Build the tracked resource
	newResource.TrackedResource = v1.TrackedResource{
		ID:   path,
		Name: id.Name(),
	}

	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldResourceGroup, id)
	logger = ucplog.GetLogger(ctx)

	existingResource := rest.ResourceGroup{}
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

	restResp := rest.NewOKResponse(versioned)
	if rgExists {
		logger.Info(fmt.Sprintf("Updated resource group %s successfully", newResource.TrackedResource.ID))
	} else {
		logger.Info(fmt.Sprintf("Created resource group %s successfully", newResource.TrackedResource.ID))
	}
	return restResp, nil
}
