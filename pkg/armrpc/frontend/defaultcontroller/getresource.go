// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/rp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// GetResource is the controller implementation to get a resource.
type GetResource[T conv.DataModelInterface] struct {
	ctrl.BaseController
	converter ToVersionedModel[T]
}

// NewGetResource creates a new GetResource.
func NewGetResource[T conv.DataModelInterface](opts ctrl.Options, convertFunc ToVersionedModel[T]) (ctrl.Controller, error) {
	return &GetResource[T]{
		BaseController: ctrl.NewBaseController(opts),
		converter:      convertFunc,
	}, nil
}

// Run executes the operation
func (e *GetResource[T]) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := new(T)
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	versioned, _ := e.converter(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
