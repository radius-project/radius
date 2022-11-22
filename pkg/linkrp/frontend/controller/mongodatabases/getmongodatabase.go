// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mongodatabases

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetMongoDatabase)(nil)

// GetMongoDatabase is the controller implementation to get the mongoDatabase conenctor resource.
type GetMongoDatabase struct {
	ctrl.BaseController
}

// NewGetMongoDatabase creates a new instance of GetMongoDatabase.
func NewGetMongoDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetMongoDatabase{ctrl.NewBaseController(opts)}, nil
}

func (mongo *GetMongoDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.MongoDatabase{}
	_, err := mongo.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.MongoDatabaseDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
