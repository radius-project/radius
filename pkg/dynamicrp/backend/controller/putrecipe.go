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

package controller

import (
	"context"

	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/dynamicrp/backend/processor"
	recipecontroller "github.com/radius-project/radius/pkg/portableresources/backend/controller"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
)

// RecipePutController is the async operation controller to perform PUT processing on "recipe" dynamic resources.
type RecipePutController struct {
	ctrl.BaseController
	opts                ctrl.Options
	engine              engine.Engine
	configurationLoader configloader.ConfigurationLoader
}

// NewRecipePutController creates a new RecipePutController.
func NewRecipePutController(opts ctrl.Options, engine engine.Engine, configurationLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &RecipePutController{
		BaseController:      ctrl.NewBaseAsyncController(opts),
		opts:                opts,
		engine:              engine,
		configurationLoader: configurationLoader,
	}, nil
}

// Run processes PUT operations for dynamic resources deployed using recipes.
// It creates and delegates the request to CreateOrUpdateResource controller to handle the operation.
func (c *RecipePutController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	putController, err := recipecontroller.NewCreateOrUpdateResource(c.opts, &processor.DynamicProcessor{}, c.engine, c.configurationLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	return putController.Run(ctx, request)
}
