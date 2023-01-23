// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
)

var _ ctrl.Controller = (*CreateOrUpdateMongoDatabase)(nil)
var (
	// AsyncPutContainerOperationTimeout is the default timeout duration of async put mongoDatabase operation.
	AsyncPutContainerOperationTimeout = time.Duration(10) * time.Minute
)

// CreateOrUpdateMongoDatabase is the controller implementation to create or update MongoDatabase link resource.
type CreateOrUpdateMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]
	dp deployment.DeploymentProcessor
}

// NewCreateOrUpdateMongoDatabase creates a new instance of CreateOrUpdateMongoDatabase.
func NewCreateOrUpdateMongoDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateMongoDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.MongoDatabase]{
				RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
				ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateMongoDatabase operation.
func (mongoDatabase *CreateOrUpdateMongoDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := mongoDatabase.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := mongoDatabase.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := mongoDatabase.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, mongoDatabase.Options())
	if r != nil || err != nil {
		return r, err
	}

	if r, err := mongoDatabase.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, AsyncPutContainerOperationTimeout, &etag); r != nil || err != nil {
		return r, err

	}
	return mongoDatabase.ConstructAsyncResponse(ctx, req.Method, etag, newResource)

}
