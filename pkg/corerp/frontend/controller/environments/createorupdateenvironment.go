// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateEnvironment)(nil)

// CreateOrUpdateEnvironments implements the resource types and APIs of Applications.Core resource provider.
type CreateOrUpdateEnvironment struct {
	ctrl.BaseController
}

// NewCreateOrUpdateEnvironment creates a new CreateOrUpdateEnvironment.
func NewCreateOrUpdateEnvironment(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*CreateOrUpdateEnvironment, error) {
	return &CreateOrUpdateEnvironment{
		BaseController: ctrl.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

func (e *CreateOrUpdateEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	e.Validate(ctx, req)

	// TODO: WIP
	return rest.NewOKResponse("ok"), nil
}

func (e *CreateOrUpdateEnvironment) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
