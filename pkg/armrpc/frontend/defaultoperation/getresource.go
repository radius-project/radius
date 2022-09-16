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
	conv.ResourceDataModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewGetResource creates a new GetResource controller instance.
func NewGetResource[P interface {
	*T
	conv.ResourceDataModel
}, T any](opts ctrl.Options, modelConverter conv.ConvertToAPIModel[T]) (ctrl.Controller, error) {
	return &GetResource[P, T]{
		ctrl.NewOperation[P](opts, nil, modelConverter),
	}, nil
}

// Run fetches the resource from the datastore.
func (e *GetResource[P, T]) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	resource, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
	}

	return e.ConstructSyncResponse(ctx, req.Method, etag, resource)
}
