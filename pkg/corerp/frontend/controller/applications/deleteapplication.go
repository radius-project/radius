// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package applications

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

var _ ctrl.Controller = (*DeleteApplication)(nil)

// DeleteApplication is the controller implementation to delete application resource.
type DeleteApplication struct {
	ctrl.Operation[*rm, rm]
}

// NewDeleteApplication creates a new DeleteApplication.
func NewDeleteApplication(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteApplication{
		ctrl.NewOperation(opts, converter.ApplicationDataModelFromVersioned, converter.ApplicationDataModelToVersioned),
	}, nil
}

func (a *DeleteApplication) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := a.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if err := a.ValidateResource(ctx, req, nil, old, etag); err != nil {
		return nil, err
	}

	if err := a.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
