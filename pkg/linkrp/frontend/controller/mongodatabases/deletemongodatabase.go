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
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteMongoDatabase)(nil)

// DeleteMongoDatabase is the controller implementation to delete mongoDatabase link resource.
type DeleteMongoDatabase struct {
	ctrl.Operation[*datamodel.MongoDatabase, datamodel.MongoDatabase]
	dp deployment.DeploymentProcessor
}

// NewDeleteMongoDatabase creates a new instance DeleteMongoDatabase.
func NewDeleteMongoDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteMongoDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.MongoDatabase]{
				RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
				ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
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

	r, err := mongoDatabase.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	err = mongoDatabase.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: old, OutputResources: old.Properties.Status.OutputResources, ComputedValues: old.ComputedValues, SecretValues: old.SecretValues, RecipeData: old.RecipeData})
	if err != nil {
		return nil, err
	}

	err = mongoDatabase.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
