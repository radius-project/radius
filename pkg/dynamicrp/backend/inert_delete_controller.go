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

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
)

// InertDeleteController is the async operation controller to perform DELETE processing on "inert" dynamic resources.
type InertDeleteController struct {
	ctrl.BaseController
}

// NewInertDeleteController creates a new InertDeleteController.
func NewInertDeleteController(opts ctrl.Options) (ctrl.Controller, error) {
	return &InertDeleteController{
		BaseController: ctrl.NewBaseAsyncController(opts),
	}, nil
}

// Run implements the async controller interface.
func (c *InertDeleteController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	err := c.DatabaseClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
