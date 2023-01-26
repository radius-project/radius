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
)

var _ ctrl.Controller = (*DeleteMongoDatabase)(nil)

var (
	// AsyncDeleteMongoDatabaseOperationTimeout is the default timeout duration of async delete container operation.
	AsyncDeleteMongoDatabaseOperationTimeout = time.Duration(900) * time.Second
)

// DeleteMongoDatabase is the controller implementation to delete mongoDatabase link resource.
type DeleteMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]
}

// NewDeleteMongoDatabase creates a new instance DeleteMongoDatabase.
func NewDeleteMongoDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteMongoDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.MongoDatabase]{
				RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
				ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
			}),
	}, nil
}

func (mongoDatabase *DeleteMongoDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := mongoDatabase.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	r, err := mongoDatabase.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	if r, err := mongoDatabase.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, AsyncDeleteMongoDatabaseOperationTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return mongoDatabase.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
