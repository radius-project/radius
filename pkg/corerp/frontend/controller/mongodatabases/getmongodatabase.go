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
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ controller.ControllerInterface = (*GetMongoDatabase)(nil)

// GetMongoDatabase controller implementation to get mongoDatabase resource
type GetMongoDatabase struct {
	controller.BaseController
}

func NewGetMongoDatabase(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*GetMongoDatabase, error) {
	return &GetMongoDatabase{
		BaseController: controller.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

func (e *GetMongoDatabase) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	rID := serviceCtx.ResourceID

	// TODO: Placeholder for now, update to retrieve the resource from data store
	resourceModel := &datamodel.MongoDatabase{
		TrackedResource: datamodel.TrackedResource{
			ID:       rID.ID,
			Name:     rID.Name(),
			Type:     rID.Type(),
			Location: "West US",
		},
		SystemData: *serviceCtx.SystemData(),
		Properties: datamodel.MongoDatabaseProperties{
			Application: "placeholdeRadiusAppID",
			FromResource: datamodel.FromResource{
				Source: "placeholderCosmosMongoDBID",
			},
		},
	}

	versioned, _ := converter.MongoDatabaseDataModelToVersioned(resourceModel, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
