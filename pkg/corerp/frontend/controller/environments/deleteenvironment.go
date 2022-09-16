// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteEnvironment)(nil)

// DeleteEnvironment is the controller implementation to delete environment resource.
type DeleteEnvironment struct {
	ctrl.Operation[*rm, rm]
}

// NewDeleteEnvironment creates a new DeleteEnvironment.
func NewDeleteEnvironment(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteEnvironment{
		ctrl.NewOperation(opts, converter.EnvironmentDataModelFromVersioned, converter.EnvironmentDataModelToVersioned),
	}, nil
}

func (e *DeleteEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r := e.ValidateResource(ctx, req, nil, old, etag); r != nil {
		return r, nil
	}

	if err := e.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
