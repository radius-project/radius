// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package resourcegroups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	http "net/http"

	"github.com/project-radius/radius/pkg/middleware"
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

	var rg rest.ResourceGroup
	err = json.Unmarshal(body, &rg)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	rg.ID = path
	rgExists := true
	ID, err := resources.Parse(rg.ID)
	//cannot parse ID something wrong with request
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldResourceGroup, rg.ID)
	logger := ucplog.GetLogger(ctx)

	existingRG := rest.ResourceGroup{}
	etag, err := r.GetResource(ctx, ID.String(), &existingRG)
	if err != nil {
		if errors.Is(err, &store.ErrNotFound{}) {
			rgExists = false
			logger.Info(fmt.Sprintf("No existing resource group %s found in db", ID))
		} else {
			return nil, err
		}
	}

	rg.Name = ID.Name()
	_, err = r.SaveResource(ctx, ID.String(), rg, etag)
	if err != nil {
		return nil, err
	}

	restResp := rest.NewOKResponse(rg)
	if rgExists {
		logger.Info(fmt.Sprintf("Updated resource group %s successfully", rg.Name))
	} else {
		logger.Info(fmt.Sprintf("Created resource group %s successfully", rg.Name))
	}
	return restResp, nil
}
