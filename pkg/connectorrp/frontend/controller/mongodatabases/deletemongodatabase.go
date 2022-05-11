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
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ base_ctrl.ControllerInterface = (*DeleteMongoDatabase)(nil)

// DeleteMongoDatabase is the controller implementation to delete mongodatabase connector resource.
type DeleteMongoDatabase struct {
	base_ctrl.BaseController
}

// NewDeleteMongoDatabase creates a new instance DeleteMongoDatabase.
func NewDeleteMongoDatabase(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &DeleteMongoDatabase{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (mongo *DeleteMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	// Read resource metadata from the storage
	existingResource := &datamodel.MongoDatabase{}
	etag, err := mongo.GetResource(ctx, serviceCtx.ResourceID.ID, existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	err = base_ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.ID, err.Error()), nil
	}

	err = mongo.DBClient.Delete(ctx, serviceCtx.ResourceID.ID)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
