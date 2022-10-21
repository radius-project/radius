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
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
)

var _ ctrl.Controller = (*DeleteMongoDatabase)(nil)

var (
	// AsyncDeleteMongoDatabaseOperationTimeout is the default timeout duration of async delete container operation.
	AsyncDeleteMongoDatabaseOperationTimeout = time.Duration(300) * time.Second
)

// DeleteMongoDatabase is the controller implementation to delete mongodatabase connector resource.
type DeleteMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]
}

// NewDeleteMongoDatabase creates a new instance DeleteMongoDatabase.
func NewDeleteMongoDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteMongoDatabase{
		ctrl.NewOperation(opts, ctrl.ResourceOptions[datamodel.MongoDatabase]{
			RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
			ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
		}),
	}, nil
}

func (mongo *DeleteMongoDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := mongo.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := mongo.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if r, err := mongo.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, AsyncDeleteMongoDatabaseOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return mongo.ConstructAsyncResponse(ctx, req.Method, etag, old)

}
