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
	datamodel "github.com/project-radius/radius/pkg/corerp/datamodel"
	converter "github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	validation "github.com/project-radius/radius/pkg/corerp/datamodel/validation"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteApplication)(nil)

// DeleteApplication is the controller implementation to delete application resource.
type DeleteApplication struct {
	ctrl.Operation[*datamodel.Application, datamodel.Application]
}

// NewDeleteApplication creates a new DeleteApplication.
func NewDeleteApplication(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteApplication{
		ctrl.NewOperation(opts, converter.ApplicationDataModelFromVersioned, converter.ApplicationDataModelToVersioned, validation.NewApplicationResourceValidators()),
	}, nil
}

func (a *DeleteApplication) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := a.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := a.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	if err := a.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
