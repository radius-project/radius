// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ ctrl.ControllerInterface = (*GetEnvironment)(nil)

// GetEnvironment is the controller implementation to get the environments resource.
type GetEnvironment struct {
	ctrl.BaseController
}

// NewGetEnvironment creates a new GetEnvironment.
func NewGetEnvironment(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &GetEnvironment{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (e *GetEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
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
	return rest.NewOKResponse(versioned), nil
}
