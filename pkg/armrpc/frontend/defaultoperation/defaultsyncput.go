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

// DefaultSyncPut is the controller implementation to create or update resource synchronously.
type DefaultSyncPut[P interface {
	*T
	v1.ResourceDataModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewDefaultSyncPut creates a new DefaultSyncPut.
//
// # Function Explanation
// 
//	The DefaultSyncPut function creates a new DefaultSyncPut controller and returns it, or an error if one occurs. It takes 
//	in two parameters, opts and resourceOpts, and uses them to create a new Operation.
func NewDefaultSyncPut[P interface {
	*T
	v1.ResourceDataModel
}, T any](opts ctrl.Options, resourceOpts ctrl.ResourceOptions[T]) (ctrl.Controller, error) {
	return &DefaultSyncPut[P, T]{ctrl.NewOperation[P](opts, resourceOpts)}, nil
}

// Run executes DefaultSyncPut operation.
//
// # Function Explanation
// 
//	The DefaultSyncPut function runs a series of filters to validate and update a resource, then saves the resource and 
//	returns a response with the new etag. If any of the filters or the save operation fail, an error is returned.
func (e *DefaultSyncPut[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
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

	P(newResource).SetProvisioningState(v1.ProvisioningStateSucceeded)
	newEtag, err := e.SaveResource(ctx, serviceCtx.ResourceID.String(), newResource, etag)
	if err != nil {
		return nil, err
	}

	return e.ConstructSyncResponse(ctx, req.Method, newEtag, newResource)
}
