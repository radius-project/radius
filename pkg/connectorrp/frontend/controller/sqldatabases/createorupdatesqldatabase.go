// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateSqlDatabase)(nil)

// CreateOrUpdateSqlDatabase is the controller implementation to create or update SqlDatabase connector resource.
type CreateOrUpdateSqlDatabase struct {
	ctrl.BaseController
}

// NewCreateOrUpdateSqlDatabase creates a new instance of CreateOrUpdateSqlDatabase.
func NewCreateOrUpdateSqlDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateSqlDatabase{ctrl.NewBaseController(opts)}, nil
}

// Run executes CreateOrUpdateSqlDatabase operation.
func (sql *CreateOrUpdateSqlDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := sql.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	rendererOutput, err := sql.DeploymentProcessor().Render(ctx, serviceCtx.ResourceID, newResource)
	if err != nil {
		return nil, err
	}
	deploymentOutput, err := sql.DeploymentProcessor().Deploy(ctx, serviceCtx.ResourceID, rendererOutput)
	if err != nil {
		return nil, err
	}

	newResource.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.Resources
	newResource.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
	newResource.InternalMetadata.SecretValues = deploymentOutput.SecretValues

	// Read existing resource info from the data store
	existingResource := &datamodel.SqlDatabase{}
	etag, err := sql.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if req.Method == http.MethodPatch && err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	// Add system metadata to requested resource
	newResource.SystemData = ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
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
	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.SqlDatabaseDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.String()
	dm.TrackedResource = ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = v1.ProvisioningStateSucceeded

	return dm, nil
}
