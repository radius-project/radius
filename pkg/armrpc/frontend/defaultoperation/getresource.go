// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"net/http"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

// GetResource is the controller implementation to get a resource.
type GetResource[P interface {
	*T
	conv.DataModelInterface
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewGetResource creates a new GetResource controller instance.
func NewGetResource[P interface {
	*T
	conv.DataModelInterface
}, T any](opts ctrl.Options, outputConverter conv.ResponseConverter[T]) (ctrl.Controller, error) {
	return &GetResource[P, T]{
		ctrl.NewOperation[P](opts, nil, outputConverter),
	}, nil
}

// Run fetches the resource from the datastore.
func (e *GetResource[P, T]) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	resource, etag, isNew, err := e.GetResourceFromStore(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if isNew {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return e.ConstructSyncResponse(ctx, req.Method, etag, resource)
}
