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
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ctrl.Controller = (*DeleteSqlDatabase)(nil)

// DeleteSqlDatabase is the controller implementation to delete sqldatabase link resource.
type DeleteSqlDatabase struct {
	ctrl.Operation[*datamodel.SqlDatabase, datamodel.SqlDatabase]

	KubeClient runtimeclient.Client
	dp         deployment.DeploymentProcessor
}

// NewDeleteSqlDatabase creates a new instance DeleteSqlDatabase.
func NewDeleteSqlDatabase(opts fctrl.Options) (ctrl.Controller, error) {
	return &DeleteSqlDatabase{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.SqlDatabase]{
				RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
				ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
			}),
		KubeClient: opts.KubeClient,
		dp:         opts.DeployProcessor,
	}, nil
}

func (sql *DeleteSqlDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := sql.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := sql.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	err = sql.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: old, OutputResources: old.Properties.Status.OutputResources, ComputedValues: old.ComputedValues, SecretValues: old.SecretValues, RecipeData: old.RecipeData})
	if err != nil {
		return nil, err
	}

	if err := sql.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
