// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environments

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

var _ ctrl.Controller = (*GetEnvironment)(nil)

// GetEnvironment is the controller implementation to get the environments resource.
type GetEnvironment struct {
	ctrl.BaseController
}

// NewGetEnvironment creates a new GetEnvironment.
func NewGetEnvironment(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetEnvironment{ctrl.NewBaseController(opts)}, nil
}

func (e *GetEnvironment) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.Environment{}
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	versioned, _ := converter.EnvironmentDataModelToVersioned(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
