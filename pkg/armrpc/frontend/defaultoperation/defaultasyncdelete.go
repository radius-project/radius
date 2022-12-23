// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package defaultoperation

import (
	"context"
	"net/http"
	"time"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
)

var (
	// defaultAsyncDeleteTimeout is the default timeout duration of async delete operation.
	defaultAsyncDeleteTimeout = time.Duration(120) * time.Second
)

// DefaultAsyncDelete is the controller implementation to delete async resource.
type DefaultAsyncDelete[P interface {
	*T
	conv.ResourceDataModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewDefaultAsyncDelete creates a new DefaultAsyncDelete.
func NewDefaultAsyncDelete[P interface {
	*T
	conv.ResourceDataModel
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

	if r, err := e.PrepareAsyncOperation(ctx, old, v1.ProvisioningStateAccepted, defaultAsyncDeleteTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, old)
}
