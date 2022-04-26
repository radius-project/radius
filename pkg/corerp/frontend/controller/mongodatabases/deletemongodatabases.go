// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ controller.ControllerInterface = (*DeleteMongoDatabase)(nil)

// DeleteMongoDatabase controller implementation to delete MongoDatabase resource
type DeleteMongoDatabase struct {
	controller.BaseController
}

func NewDeleteMongoDatabase(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*DeleteMongoDatabase, error) {
	return &DeleteMongoDatabase{
		BaseController: controller.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

func (e *DeleteMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	_ = e.Validate(ctx, req)
	// TODO: Delete MongoDatabase from the data store
	return rest.NewOKResponse("deleted successfully"), nil
}

func (e *DeleteMongoDatabase) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
