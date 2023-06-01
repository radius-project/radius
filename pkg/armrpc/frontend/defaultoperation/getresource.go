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

// GetResource is the controller implementation to get a resource.
type GetResource[P interface {
	*T
	v1.ResourceDataModel
}, T any] struct {
	ctrl.Operation[P, T]
}

// NewGetResource creates a new GetResource controller instance.
//
// # Function Explanation
// 
//	The NewGetResource function creates a new GetResource controller and returns it, or an error if one occurs. It takes in 
//	two parameters, an Options object and a ResourceOptions object, and handles any errors that may occur during the 
//	creation of the controller.
func NewGetResource[P interface {
	*T
	v1.ResourceDataModel
}, T any](opts ctrl.Options, resourceOpts ctrl.ResourceOptions[T]) (ctrl.Controller, error) {
	return &GetResource[P, T]{
		ctrl.NewOperation[P](opts, resourceOpts),
	}, nil
}

// Run fetches the resource from the datastore.
//
// # Function Explanation
// 
//	The GetResource function retrieves a resource from the context, checks if it exists, and returns a response with the 
//	resource and an ETag. If the resource is not found, a NotFoundResponse is returned. If an error occurs, it is returned 
//	to the caller.
func (e *GetResource[P, T]) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
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
