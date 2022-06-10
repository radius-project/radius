// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ base_ctrl.ControllerInterface = (*CreateOrUpdateApplication)(nil)

// CreateOrUpdateApplication is the controller implementation to create or update application resource.
type CreateOrUpdateApplication struct {
	base_ctrl.BaseController
}

// NewCreateOrUpdateApplication creates a new instance of CreateOrUpdateApplication.
func NewCreateOrUpdateApplication(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &CreateOrUpdateApplication{
		BaseController: ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run executes CreateOrUpdateApplication operation.
func (app *CreateOrUpdateApplication) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := app.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// Read existing application resource info from the data store
	existingResource := &datamodel.Application{}

	etag, err := app.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if req.Method == http.MethodPatch && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	UpdateExistingResourceData(ctx, existingResource, newResource)

	nr, err := app.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.ApplicationDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": nr.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (app *CreateOrUpdateApplication) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.Application, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := base_ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.ApplicationDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = base_ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = basedatamodel.ProvisioningStateSucceeded
	return dm, nil
}

// UpdateExistingResourceData updates the application resource before it is saved to the DB.
func UpdateExistingResourceData(ctx context.Context, er *datamodel.Application, nr *datamodel.Application) {
	sc := servicecontext.ARMRequestContextFromContext(ctx)
	nr.SystemData = ctrl.UpdateSystemData(er.SystemData, *sc.SystemData())
	if er.InternalMetadata.CreatedAPIVersion != "" {
		nr.InternalMetadata.CreatedAPIVersion = er.InternalMetadata.CreatedAPIVersion
	}
	nr.InternalMetadata.TenantID = sc.HomeTenantID
}
