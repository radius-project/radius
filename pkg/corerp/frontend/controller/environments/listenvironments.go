// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*ListEnvironments)(nil)

// ListEnvironments is the controller implementation to get the list of environments resources in resource group.
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
	_ = e.Validate(ctx, req)
	rID := serviceCtx.ResourceID

	// TODO: Get the environment resource from datastorage. now return fake data.

	m := &datamodel.Environment{
		TrackedResource: datamodel.TrackedResource{
			ID:       rID.ID,
			Name:     rID.Name(),
			Type:     rID.Type(),
			Location: "West US",
		},
		SystemData: *serviceCtx.SystemData(),
		Properties: datamodel.EnvironmentProperties{
			Compute: datamodel.EnvironmentCompute{
				Kind:       datamodel.KubernetesComputeKind,
				ResourceID: "fakeID",
			},
		},
	}

	versioned, _ := converter.EnvironmentDataModelToVersioned(m, serviceCtx.APIVersion)

	pagination := armrpcv1.PaginatedList{
		Value: []interface{}{versioned},
	}
	return rest.NewOKResponse(pagination), nil
}

func (e *ListEnvironments) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
