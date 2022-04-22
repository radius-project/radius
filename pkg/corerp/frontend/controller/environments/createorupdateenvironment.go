// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"

	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateEnvironment)(nil)

// CreateOrUpdateEnvironments is the controller implementation to create or update environment resource.
type CreateOrUpdateEnvironment struct {
	ctrl.BaseController
}

// NewCreateOrUpdateEnvironment creates a new CreateOrUpdateEnvironment.
func NewCreateOrUpdateEnvironment(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &CreateOrUpdateEnvironment{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run exexcutes CreateOrUpdateEnvironment operation.
func (e *CreateOrUpdateEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	existingResource := &datamodel.Environment{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.ID, existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	newResource.SystemData = ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())

	// TODO: Read Systemdata from the existing resource and update it properly.

	err = e.SaveResource(ctx, serviceCtx.ResourceID.ID, newResource, etag)
	if err != nil {
		return nil, err
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
	serviceOpt := hostoptions.FromContext(ctx)

	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	// TODO: Validate incoming request payload.
	newVersioned, err := converter.EnvironmentDataModelFromVersioned(content, apiVersion)
	newVersioned.ID = serviceCtx.ResourceID.ID
	newVersioned.TrackedResource.ID = serviceCtx.ResourceID.ID
	newVersioned.TrackedResource.Name = serviceCtx.ResourceID.Name()
	newVersioned.TrackedResource.Type = serviceCtx.ResourceID.Type()
	newVersioned.TrackedResource.Location = serviceOpt.CloudEnv.RoleLocation

	// TODO: Update the state.
	newVersioned.Properties.ProvisioningState = datamodel.ProvisioningStateSucceeded

	return newVersioned, err
}
