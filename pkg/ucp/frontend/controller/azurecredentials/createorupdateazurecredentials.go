// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------
package azurecredentials

import (
	"context"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/rest"
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

	return nil, nil
}
