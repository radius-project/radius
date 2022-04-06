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

var _ ctrl.ControllerInterface = (*ListEnvironments)(nil)

// ListEnvironments implements the resource types and APIs of Applications.Core resource provider.
type ListEnvironments struct {
	ctrl.BaseController
}

// NewListEnvironments creates a new ListEnvironments.
func NewListEnvironments(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*ListEnvironments, error) {
	return &ListEnvironments{
		BaseController: ctrl.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

func (e *ListEnvironments) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	e.Validate(ctx, req)

	// TODO: WIP
	return rest.NewOKResponse("ok"), nil
}

func (e *ListEnvironments) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
