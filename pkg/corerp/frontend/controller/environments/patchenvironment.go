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

var _ ctrl.ControllerInterface = (*PatchEnvironment)(nil)

// PatchEnvironment is the controller implementation to patch an environment resource.
type PatchEnvironment struct {
	ctrl.BaseController
}

// NewPatchEnvironment creates a new PatchEnvironment.
func NewPatchEnvironment(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (ctrl.ControllerInterface, error) {
	return &PatchEnvironment{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run executes PatchEnvironment operation.
func (e *PatchEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// Read resource metadata from the storage
	existingResource := &datamodel.Environment{}
	etag, err := e.GetResource(ctx, serviceCtx.ResourceID.ID, existingResource)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(err.Error()), nil
	}

	newResource.SystemData = ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
	if existingResource.CreatedAPIVersion != "" {
		newResource.CreatedAPIVersion = existingResource.CreatedAPIVersion
	}
	newResource.TenantID = serviceCtx.HomeTenantID

	ur, err := e.SaveResource(ctx, serviceCtx.ResourceID.ID, newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.EnvironmentDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": ur.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validate the properties.
func (e *PatchEnvironment) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.Environment, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	serviceOpt := hostoptions.FromContext(ctx)
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.EnvironmentDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	// TODO: Add more validation e.g. schema, identity, etc.
	dm.ID = serviceCtx.ResourceID.ID
	dm.TrackedResource.ID = serviceCtx.ResourceID.ID
	dm.TrackedResource.Name = serviceCtx.ResourceID.Name()
	dm.TrackedResource.Type = serviceCtx.ResourceID.Type()
	dm.TrackedResource.Location = serviceOpt.CloudEnv.RoleLocation

	// TODO: Update the state.
	dm.Properties.ProvisioningState = datamodel.ProvisioningStateSucceeded

	return dm, err
}
