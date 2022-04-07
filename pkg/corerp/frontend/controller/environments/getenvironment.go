// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"

	v20220315 "github.com/project-radius/radius/pkg/corerp/api/v20220315"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
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
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	e.Validate(ctx, req)
	rID := serviceCtx.ResourceID
	// TODO: Get the environment resource from datastorage. now return fake data.
	m := &v20220315.Environment{
		ID:         rID.ID,
		Name:       rID.Name(),
		Type:       rID.Type(),
		Location:   "West US",
		SystemData: *serviceCtx.SystemData(),
		Properties: v20220315.EnvironmentProperties{
			Compute: v20220315.EnvironmentCompute{
				Kind:       v20220315.KubernetesComputeKind,
				ResourceID: "fakeID",
			},
		},
	}
	return rest.NewOKResponse(m), nil
}

func (e *GetEnvironment) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
