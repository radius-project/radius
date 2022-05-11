// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"

	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
)

var _ base_ctrl.ControllerInterface = (*CreateOrUpdateMongoDatabase)(nil)

// CreateOrUpdateMongoDatabase is the controller implementation to create or update MongoDatabase connector resource.
type CreateOrUpdateMongoDatabase struct {
	base_ctrl.BaseController
}

// NewCreateOrUpdateMongoDatabase creates a new instance of CreateOrUpdateMongoDatabase.
func NewCreateOrUpdateMongoDatabase(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &CreateOrUpdateMongoDatabase{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run executes CreateOrUpdateMongoDatabase operation.
func (mongo *CreateOrUpdateMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := mongo.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// TODO Integrate with renderer/deployment processor to validate associated resource existence (if fromResource is defined)
	// and store resource properties and secrets reference

	// Read existing resource info from the data store
	existingResource := &datamodel.MongoDatabase{}
	etag, err := mongo.GetResource(ctx, serviceCtx.ResourceID.ID, existingResource)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return nil, err
	}

	err = base_ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.ID, err.Error()), nil
	}

	// Add system metadata to requested resource
	newResource.SystemData = base_ctrl.UpdateSystemData(existingResource.SystemData, *serviceCtx.SystemData())
	if existingResource.CreatedAPIVersion != "" {
		newResource.CreatedAPIVersion = existingResource.CreatedAPIVersion
	}
	newResource.TenantID = serviceCtx.HomeTenantID

	// Add/update resource in the data store
	savedResource, err := mongo.SaveResource(ctx, serviceCtx.ResourceID.ID, newResource, etag)
	if err != nil {
		return nil, err
	}

	versioned, err := converter.MongoDatabaseDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"ETag": savedResource.ETag}

	return rest.NewOKResponseWithHeaders(versioned, headers), nil
}

// Validate extracts versioned resource from request and validates the properties.
func (mongo *CreateOrUpdateMongoDatabase) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.MongoDatabase, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	content, err := base_ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}

	dm, err := converter.MongoDatabaseDataModelFromVersioned(content, apiVersion)
	if err != nil {
		return nil, err
	}

	dm.ID = serviceCtx.ResourceID.ID
	dm.TrackedResource = base_ctrl.BuildTrackedResource(ctx)
	dm.Properties.ProvisioningState = datamodel.ProvisioningStateSucceeded

	return dm, nil
}
