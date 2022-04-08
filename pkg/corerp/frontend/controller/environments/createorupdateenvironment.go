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
	"github.com/project-radius/radius/pkg/corerp/frontend/util"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateEnvironment)(nil)

// CreateOrUpdateEnvironments is the controller implementation to create or update environment resource.
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

// Run exexcutes CreateOrUpdateEnvironment operation.
func (e *CreateOrUpdateEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
	}

	// TODO: Save the resource and queue the async task.
	versioned, err := converter.EnvironmentDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	return rest.NewOKResponse(versioned), nil
}

// Validate extracts versioned resource from request and validate the properties.
func (e *CreateOrUpdateEnvironment) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.Environment, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	content, err := util.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}
	newVersioned, err := converter.EnvironmentDataModelFromVersioned(content, apiVersion)

	// TODO: Validate incoming request payload.
	// TODO: Read resource metadata from datastorage.
	// TODO: Read Systemdata from the existing resource and update it properly.
	newVersioned.SystemData = *serviceCtx.SystemData()
	// TODO: Update the state.
	newVersioned.Properties.ProvisioningState = datamodel.ProvisioningStateSucceeded

	return newVersioned, err
}
