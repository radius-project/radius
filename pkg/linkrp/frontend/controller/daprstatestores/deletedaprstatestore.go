// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprstatestores

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteDaprStateStore)(nil)

// DeleteDaprStateStore is the controller implementation to delete daprStateStore link resource.
type DeleteDaprStateStore struct {
	ctrl.BaseController
}

// NewDeleteDaprStateStore creates a new instance DeleteDaprStateStore.
func NewDeleteDaprStateStore(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprStateStore{ctrl.NewBaseController(opts)}, nil
}

func (daprStateStore *DeleteDaprStateStore) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	// Read resource metadata from the storage
	existingResource := &datamodel.DaprStateStore{}
	etag, err := daprStateStore.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	err = ctrl.ValidateETag(*serviceCtx, etag)
	if err != nil {
		return rest.NewPreconditionFailedResponse(serviceCtx.ResourceID.String(), err.Error()), nil
	}

	err = daprStateStore.DeploymentProcessor().Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: existingResource, OutputResources: existingResource.Properties.Status.OutputResources, ComputedValues: existingResource.ComputedValues, SecretValues: existingResource.SecretValues, RecipeData: existingResource.RecipeData})
	if err != nil {
		return nil, err
	}

	err = daprStateStore.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
