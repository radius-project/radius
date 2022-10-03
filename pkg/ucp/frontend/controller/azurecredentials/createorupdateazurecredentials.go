// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package azurecredentials

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/project-radius/radius/pkg/middleware"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*CreateOrUpdateAzureCredentials)(nil)

// CreateOrUpdateAzureCredentials is the controller implementation to create/update an azure credentials for a plane.
type CreateOrUpdateAzureCredentials struct {
	ctrl.BaseController
}

// NewCreateOrUpdateAzureCredentials creates a new CreateOrUpdateAzureCredentials.
func NewCreateOrUpdateAzureCredentials(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateAzureCredentials{ctrl.NewBaseController(opts)}, nil
}

func (r *CreateOrUpdateAzureCredentials) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	path := middleware.GetRelativePath(r.Options.BasePath, req.URL.Path)
	body, err := ctrl.ReadRequestBody(req)
	if err != nil {
		return nil, err
	}
	var aps rest.AzureProviderSecrets
	err = json.Unmarshal(body, &aps)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), nil
	}

	_, err = r.SaveResource(ctx, path, aps, "")
	if err != nil {
		return nil, err
	}

	restResp := rest.NewOKResponse(aps)
	ctx = ucplog.WrapLogContext(ctx, ucplog.LogFieldAzureCredentials, path)
	logger := ucplog.GetLogger(ctx)

	logger.Info("Created resource group default successfully")

	return restResp, nil
}
