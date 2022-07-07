// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package daprinvokehttproutes

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteDaprInvokeHttpRoute)(nil)

// DeleteDaprInvokeHttpRoute is the controller implementation to delete daprInvokeHttpRoute connector resource.
type DeleteDaprInvokeHttpRoute struct {
	ctrl.BaseController
}

// NewDeleteDaprInvokeHttpRoute creates a new instance DeleteDaprInvokeHttpRoute.
func NewDeleteDaprInvokeHttpRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprInvokeHttpRoute{ctrl.NewBaseController(opts)}, nil
}

func (daprHttpRoute *DeleteDaprInvokeHttpRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	// Read resource metadata from the storage
	existingResource := &datamodel.DaprInvokeHttpRoute{}
	etag, err := daprHttpRoute.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
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

	err = daprHttpRoute.DeploymentProcessor().Delete(ctx, serviceCtx.ResourceID, existingResource.Properties.Status.OutputResources)
	if err != nil {
		return nil, err
	}

	err = daprHttpRoute.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
