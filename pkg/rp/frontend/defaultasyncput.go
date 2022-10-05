// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"
	"net/http"
	"time"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/rp"
)

var (
	// defaultAsyncPutTimeout is the default timeout duration of async put operation.
	defaultAsyncPutTimeout = time.Duration(120) * time.Second
)

// DefaultAsyncPut is the controller implementation to create or update async resource.
type DefaultAsyncPut[P interface {
	*T
	rp.RadiusResourceModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewDefaultAsyncPut creates a new DefaultAsyncPut.
func NewDefaultAsyncPut[P interface {
	*T
	rp.RadiusResourceModel
}, T any](opts ctrl.Options, reqconv conv.ConvertToDataModel[T], respconv conv.ConvertToAPIModel[T]) (ctrl.Controller, error) {
	return &DefaultAsyncPut[P, T]{ctrl.NewOperation[P](opts, reqconv, respconv)}, nil
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

	if r, err := PrepareRadiusResource[P](ctx, old, newResource); r != nil || err != nil {
		return r, err
	}

	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, defaultAsyncPutTimeout, &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
