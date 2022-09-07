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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// GetResource is the controller implementation to get a resource.
type GetResource[P interface {
	*T
	conv.DataModelInterface
}, T any] struct {
	ctrl.BaseController
	outputConverter conv.ResponseConverter[T]
}

// NewGetResource creates a new GetResource controller instance.
func NewGetResource[P interface {
	*T
	conv.DataModelInterface
}, T any](opts ctrl.Options, outputConverter conv.ResponseConverter[T]) (ctrl.Controller, error) {
	return &GetResource[P, T]{
		BaseController:  ctrl.NewBaseController(opts),
		outputConverter: outputConverter,
	}, nil
}

// Run fetches the resource from the datastore.
func (e *GetResource[P, T]) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	existingResource := new(T)
	_, err := e.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil && errors.Is(&store.ErrNotFound{}, err) {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	versioned, _ := e.outputConverter(existingResource, serviceCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
