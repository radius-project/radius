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
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetSqlDatabase)(nil)

// GetSqlDatabase is the controller implementation to get the sqlDatabse conenctor resource.
type GetSqlDatabase struct {
	ctrl.BaseController
}

// NewGetSqlDatabase creates a new instance of GetSqlDatabase.
func NewGetSqlDatabase(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetSqlDatabase{ctrl.NewBaseController(opts)}, nil
}

func (sql *GetSqlDatabase) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.SqlDatabase{}
	_, err := sql.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.SqlDatabaseDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
