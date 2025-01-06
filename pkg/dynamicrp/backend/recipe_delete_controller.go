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
	recipecontroller "github.com/radius-project/radius/pkg/portableresources/backend/controller"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
)

// RecipeDeleteController is the async operation controller to perform DELETE processing on "recipe" dynamic resources.
type RecipeDeleteController struct {
	ctrl.BaseController
	opts                ctrl.Options
	engine              engine.Engine
	configurationLoader configloader.ConfigurationLoader
}

// NewRecipeDeleteController creates a new RecipeDeleteController.
func NewRecipeDeleteController(opts ctrl.Options, engine engine.Engine, configurationLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &RecipeDeleteController{
		BaseController:      ctrl.NewBaseAsyncController(opts),
		opts:                opts,
		engine:              engine,
		configurationLoader: configurationLoader,
	}, nil
}

// Run implements the async controller interface.
func (c *RecipeDeleteController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	wrapped, err := recipecontroller.NewDeleteResource(c.opts, &dynamicProcessor{}, c.engine, c.configurationLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	return wrapped.Run(ctx, request)
}
