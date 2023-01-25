// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

// DefaultAsyncPut is the controller implementation to create or update async resource.
type DefaultAsyncPut[P interface {
	*T
	v1.ResourceDataModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewDefaultAsyncPut creates a new DefaultAsyncPut.
func NewDefaultAsyncPut[P interface {
	*T
	v1.ResourceDataModel
}, T any](opts ctrl.Options, resourceOpts ctrl.ResourceOptions[T]) (ctrl.Controller, error) {
	return &DefaultAsyncPut[P, T]{ctrl.NewOperation[P](opts, resourceOpts)}, nil
}

// Run executes DefaultAsyncPut operation.
func (e *DefaultAsyncPut[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	newResource, err := e.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := e.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if r, err := e.PrepareResource(ctx, req, newResource, old, etag); r != nil || err != nil {
		return r, err
	}

	for _, filter := range e.UpdateFilters() {
		if resp, err := filter(ctx, newResource, old, e.Options()); resp != nil || err != nil {
			return resp, err
		}
	}
	tmp := e.AsyncOperationTimeout()
	fmt.Printf(tmp.String())
	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, e.AsyncOperationTimeout(), &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
