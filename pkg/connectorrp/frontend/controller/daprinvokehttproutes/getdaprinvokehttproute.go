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
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetDaprInvokeHttpRoute)(nil)

// GetDaprInvokeHttpRoute is the controller implementation to get the daprInvokeHttpRoute conenctor resource.
type GetDaprInvokeHttpRoute struct {
	ctrl.BaseController
}

// NewGetDaprInvokeHttpRoute creates a new instance of GetDaprInvokeHttpRoute.
func NewGetDaprInvokeHttpRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetDaprInvokeHttpRoute{ctrl.NewBaseController(opts)}, nil
}

func (daprHttpRoute *GetDaprInvokeHttpRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.DaprInvokeHttpRoute{}
	_, err := daprHttpRoute.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.DaprInvokeHttpRouteDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
