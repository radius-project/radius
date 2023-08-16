/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package defaultoperation

import (
	"context"
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

// Run executes asynchronous create or update operation by validating new resource metadata, ensuring if it is new resource
// or updated resource, running custom update filters, and queuing async operation and returns an async response.
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

	if r, err := e.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, e.AsyncOperationTimeout(), &etag); r != nil || err != nil {
		return r, err
	}

	return e.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
