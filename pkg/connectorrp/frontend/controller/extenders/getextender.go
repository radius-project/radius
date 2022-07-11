// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package extenders

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

var _ ctrl.Controller = (*GetExtender)(nil)

// GetExtender is the controller implementation to get the extender conenctor resource.
type GetExtender struct {
	ctrl.BaseController
}

// NewGetExtender creates a new instance of GetExtender.
func NewGetExtender(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetExtender{ctrl.NewBaseController(opts)}, nil
}

func (extender *GetExtender) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.ExtenderResponse{}
	_, err := extender.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.ExtenderDataModelToVersioned(existingResource, serviceCtx.APIVersion, false)
	return rest.NewOKResponse(versioned), nil
}
