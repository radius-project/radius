// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/corerp/api/armrpcv1"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

var _ controller.ControllerInterface = (*ListMongoDatabases)(nil)

// ListMongoDatabases controller implementation to get the list of MongoDatabases resources in the resource group
type ListMongoDatabases struct {
	controller.BaseController
}

func NewListMongoDatabases(db db.RadrpDB, jobEngine deployment.DeploymentProcessor) (*ListMongoDatabases, error) {
	return &ListMongoDatabases{
		BaseController: controller.BaseController{
			DBProvider: db,
			JobEngine:  jobEngine,
		},
	}, nil
}

func (e *ListMongoDatabases) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)
	_ = e.Validate(ctx, req)
	rID := serviceCtx.ResourceID

	// TODO: Placeholder for now, update to retrieve resources from data store
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

	pagination := armrpcv1.PaginatedList{
		Value: []interface{}{versioned},
	}
	return rest.NewOKResponse(pagination), nil
}

func (e *ListMongoDatabases) Validate(ctx context.Context, req *http.Request) error {
	return nil
}
