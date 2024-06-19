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

package embedded

import (
	"context"
	"fmt"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
)

var _ ctrl.Controller = (*Controller)(nil)

// Controller is the async operation controller to perform background processing on tracked resources.
type Controller struct {
	ctrl.BaseController
}

// NewController creates a new Controller controller which is used to process resources asynchronously.
func NewController(opts ctrl.Options) (ctrl.Controller, error) {
	return &Controller{
		BaseController: ctrl.NewBaseAsyncController(opts),
	}, nil
}

// Run implements the async operation controller to process resources asynchronously.
func (c *Controller) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	operationType, _ := v1.ParseOperationType(request.OperationType)
	switch operationType.Method {
	case http.MethodPut:
		return c.processPut(ctx, request)
	case http.MethodDelete:
		return c.processDelete(ctx, request)
	default:
		e := v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("Invalid operation type: %q", operationType),
			Target:  request.ResourceID,
		}
		return ctrl.NewFailedResult(e), nil
	}
}

func (c *Controller) processDelete(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	err := c.StorageClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (c *Controller) processPut(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}
