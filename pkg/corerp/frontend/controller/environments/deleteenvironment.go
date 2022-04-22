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
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ ctrl.ControllerInterface = (*DeleteEnvironment)(nil)

// DeleteEnvironment is the controller implementation to delete environment resource.
type DeleteEnvironment struct {
	ctrl.BaseController
}

// NewDeleteEnvironment creates a new DeleteEnvironment.
func NewDeleteEnvironment(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &DeleteEnvironment{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (e *DeleteEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	_ = e.Validate(ctx, req)
	// TODO: Delete environment from datastore.
	return rest.NewOKResponse("deleted successfully"), nil
}

func (e *DeleteEnvironment) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
