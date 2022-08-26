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
	"strings"

	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*GetResourceGroup)(nil)

// GetResourceGroup is the controller implementation to get the details of a UCP resource group
type GetResourceGroup struct {
	ctrl.BaseController
}

// NewGetResourceGroup creates a new GetResourceGroup.
func NewGetResourceGroup(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetResourceGroup{ctrl.NewBaseController(opts)}, nil
}

func (r *GetResourceGroup) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(r.Options.BasePath, req.URL.Path)
	logger := ucplog.GetLogger(ctx)
	id := strings.ToLower(path)
	resourceID, err := resources.Parse(id)
	if err != nil {
		if err != nil {
			return rest.NewBadRequestResponse(err.Error()), nil
		}
	}
	logger.Info(fmt.Sprintf("Getting resource group %s from db", resourceID))
	rg := datamodel.ResourceGroup{}
	_, err = r.GetResource(ctx, resourceID.String(), &rg)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			logger.Info(fmt.Sprintf("Resource group %s not found in db", resourceID))
			restResponse := rest.NewNotFoundResponse(path)
			return restResponse, nil
		}
		return nil, err
	}
	// Convert to version agnostic data model
	apiVersion := ctrl.GetAPIVersion(logger, req)

	// Return a versioned response of the resource group
	versioned, err := converter.ResourceGroupDataModelToVersioned(&rg, apiVersion)
	if err != nil {
		return nil, err
	}

	restResponse := rest.NewOKResponse(versioned)
	return restResponse, nil
}
