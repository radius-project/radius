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
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/store"
)

var _ base_ctrl.ControllerInterface = (*GetMongoDatabase)(nil)

// GetMongoDatabase is the controller implementation to get the mongoDatabse conenctor resource.
type GetMongoDatabase struct {
	base_ctrl.BaseController
}

// NewGetMongoDatabase creates a new instance of GetMongoDatabase.
func NewGetMongoDatabase(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &GetMongoDatabase{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

func (mongo *GetMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.MongoDatabase{}
	_, err := mongo.GetResource(ctx, serviceCtx.ResourceID.ID, existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.MongoDatabaseDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
