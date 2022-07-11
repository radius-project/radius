// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package httproutes

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/datamodel/converter"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetHTTPRoute)(nil)

// GetHTTPRoute is the controller implementation to get the HTTPRoute resource.
type GetHTTPRoute struct {
	ctrl.BaseController
}

// NewGetHTTPRoute creates a new GetHTTPRoute.
func NewGetHTTPRoute(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetHTTPRoute{ctrl.NewBaseController(opts)}, nil
}

// Run executes GetHTTPRoute operation
func (e *GetHTTPRoute) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.HTTPRoute{}
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	versioned, _ := converter.HTTPRouteDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
