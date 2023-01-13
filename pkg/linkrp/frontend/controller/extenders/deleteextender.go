// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

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

var _ ctrl.Controller = (*DeleteExtender)(nil)

// DeleteExtender is the controller implementation to delete extender link resource.
type DeleteExtender struct {
	ctrl.Operation[*datamodel.Extender, datamodel.Extender]
	dp deployment.DeploymentProcessor
}

// NewDeleteExtender creates a new instance DeleteExtender.
func NewDeleteExtender(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteExtender{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.Extender]{
				RequestConverter:  converter.ExtenderDataModelFromVersioned,
				ResponseConverter: converter.ExtenderDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

func (extender *DeleteExtender) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := extender.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	r, err := extender.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	err = extender.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: old, OutputResources: old.Properties.Status.OutputResources, ComputedValues: old.ComputedValues, SecretValues: old.SecretValues, RecipeData: old.RecipeData})
	if err != nil {
		return nil, err
	}

	err = extender.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
