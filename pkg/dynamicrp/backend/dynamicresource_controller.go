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

package backend

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// DynamicResourceController is the async operation controller to perform processing on dynamic resources.
//
// This controller will use the capabilities and the operation to determine the correct controller to use.
type DynamicResourceController struct {
	ctrl.BaseController
}

// NewDynamicResourceController creates a new DynamicResourcePutController.
func NewDynamicResourceController(opts ctrl.Options) (ctrl.Controller, error) {
	return &DynamicResourceController{
		BaseController: ctrl.NewBaseAsyncController(opts),
	}, nil
}

// Run implements the async controller interface.
func (c *DynamicResourceController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	// This is where we have the opportunity to branch out to different controllers based on:
	// - The operation type. (eg: PUT, DELETE, etc)
	// - The capabilities of the resource type. (eg: Does it support recipes?)
	controller, err := c.selectController(request)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create controller: %w", err)
	}

	return controller.Run(ctx, request)

}

func (c *DynamicResourceController) selectController(request *ctrl.Request) (ctrl.Controller, error) {
	ot, ok := v1.ParseOperationType(request.OperationType)
	if !ok {
		return nil, fmt.Errorf("invalid operation type: %q", request.OperationType)
	}

	id, err := resources.ParseResource(request.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("invalid resource ID: %q", request.ResourceID)
	}

	options := ctrl.Options{
		DatabaseClient: c.DatabaseClient(),
		ResourceType:   id.Type(),
	}

	switch ot.Method {
	case v1.OperationDelete:
		return NewInertDeleteController(options)
	case v1.OperationPut:
		return NewInertPutController(options)
	default:
		return nil, fmt.Errorf("unsupported operation type: %q", request.OperationType)
	}
}
