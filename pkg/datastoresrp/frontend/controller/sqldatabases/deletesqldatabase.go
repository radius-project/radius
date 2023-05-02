// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package sqldatabases

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	frontend_ctrl "github.com/project-radius/radius/pkg/datastoresrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteSqlDatabase)(nil)

// DeleteSqlDatabase is the controller implementation to delete sqlDatabase link resource.
type DeleteSqlDatabase struct {
	ctrl.Operation[*datamodel.SqlDatabase, datamodel.SqlDatabase]
	dp deployment.DeploymentProcessor
}

// NewDeleteSqlDatabase creates a new instance DeleteSqlDatabase.
func NewDeleteSqlDatabase(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteSqlDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.SqlDatabase]{
				RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
				ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

func (sqlDatabase *DeleteSqlDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := sqlDatabase.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	r, err := sqlDatabase.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	err = sqlDatabase.dp.Delete(ctx, serviceCtx.ResourceID, old.Properties.Status.OutputResources)
	if err != nil {
		return nil, err
	}

	err = sqlDatabase.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
