// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"

	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
)

var _ base_ctrl.ControllerInterface = (*CreateOrUpdateSqlDatabase)(nil)

// CreateOrUpdateSqlDatabase is the controller implementation to create or update SqlDatabase connector resource.
type CreateOrUpdateSqlDatabase struct {
	base_ctrl.BaseController
}

// NewCreateOrUpdateSqlDatabase creates a new instance of CreateOrUpdateSqlDatabase.
func NewCreateOrUpdateSqlDatabase(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &CreateOrUpdateSqlDatabase{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run executes CreateOrUpdateSqlDatabase operation.
func (sql *CreateOrUpdateSqlDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := sql.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// TODO Integrate with renderer/deployment processor to validate associated resource existence (if fromResource is defined)
	// and store resource properties and secrets reference

	// Read existing resource info from the data store
	existingResource := &datamodel.SqlDatabase{}
	etag, err := sql.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = base_ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	// Add system metadata to requested resource
	newResource.SystemData = base_ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
	if existingResource.CreatedAPIVersion != "" {
		newResource.CreatedAPIVersion = existingResource.CreatedAPIVersion
	}
	newResource.TenantID = serviceCtx.HomeTenantID

	// Add/update resource in the data store
	savedResource, err := sql.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.SqlDatabaseDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (sql *CreateOrUpdateSqlDatabase) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.SqlDatabase, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := base_ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.SqlDatabaseDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = base_ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = basedatamodel.ProvisioningStateSucceeded

	return dm, nil
}
