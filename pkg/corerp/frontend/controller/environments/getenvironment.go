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

var _ ctrl.ControllerInterface = (*GetEnvironment)(nil)

// GetEnvironment implements the resource types and APIs of Applications.Core resource provider.
type GetEnvironment struct {
	ctrl.BaseController
}

// NewGetEnvironment creates a new GetEnvironment.
func NewGetEnvironment(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*GetEnvironment, error) {
	return &GetEnvironment{
		BaseController: ctrl.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

func (e *GetEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	e.Validate(ctx, req)

	// TODO: WIP
	return rest.NewOKResponse("ok"), nil
}

func (e *GetEnvironment) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
