// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

// DefaultAsyncDelete is the controller implementation to delete async resource.
type DefaultAsyncDelete[P interface {
	*T
	v1.ResourceDataModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewDefaultAsyncDelete creates a new DefaultAsyncDelete.
func NewDefaultAsyncDelete[P interface {
	*T
	v1.ResourceDataModel
}, T any](opts ctrl.Options, resourceOpts ctrl.ResourceOptions[T]) (ctrl.Controller, error) {
	return &DefaultAsyncDelete[P, T]{ctrl.NewOperation[P](opts, resourceOpts)}, nil
}

// Run executes DefaultAsyncDelete operation
func (e *DefaultAsyncDelete[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := e.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	for _, filter := range e.DeleteFilters() {
		if resp, err := filter(ctx, old, e.Options()); resp != nil || err != nil {
			return resp, err
		}
	}

	if r, err := e.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, e.AsyncOperationTimeout(), &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
