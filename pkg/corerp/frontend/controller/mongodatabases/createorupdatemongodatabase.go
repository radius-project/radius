// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ ctrl.ControllerInterface = (*CreateOrUpdateMongoDatabase)(nil)

// CreateOrUpdateMongoDatabase is the controller implementation to create or update MongoDatabase resource
type CreateOrUpdateMongoDatabase struct {
	ctrl.BaseController
}

// NewCreateOrUpdateMongoDatabase creates a new MongoDatabase resource
func NewCreateOrUpdateMongoDatabase(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*CreateOrUpdateMongoDatabase, error) {
	return &CreateOrUpdateMongoDatabase{
		BaseController: ctrl.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

// Run: Executes CreateOrUpdateMongoDatabase operation
func (e *CreateOrUpdateMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	newResource, err := e.Validate(ctx, req, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	// TODO: Save the resource in data store and queue the async task
	versioned, err := converter.MongoDatabaseDataModelToVersioned(newResource, serviceCtx.APIVersion)
	if err != nil {
		return nil, err
	}

	return rest.NewOKResponse(versioned), nil
}

// Validate extracts versioned resource from request and validate the properties.
func (e *CreateOrUpdateMongoDatabase) Validate(ctx context.Context, req *http.Request, apiVersion string) (*datamodel.MongoDatabase, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	content, err := ctrl.ReadJSONBody(req)
	if err != nil {
		return nil, err
	}
	newVersioned, err := converter.MongoDatabaseDataModelFromVersioned(content, apiVersion)

	// TODO:
	// 1. Validate incoming request payload
	// 2. Read resource metadata from data store
	// 3. Read system data from existing resource and update it properly
	newVersioned.SystemData = *serviceCtx.SystemData()
	// TODO: Update state to reflect operation state
	newVersioned.Properties.ProvisioningState = datamodel.ProvisioningStateSucceeded

	return newVersioned, err
}
