// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	v20220315 "github.com/project-radius/radius/pkg/corerp/api/v20220315"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
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
	pagination := armrpcv1.PaginatedList{
		Value: []interface{}{m},
	}
	return rest.NewOKResponse(pagination), nil
}

func (e *ListEnvironments) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
