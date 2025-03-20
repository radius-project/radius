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

// RecipeDeleteController is the async operation controller to perform DELETE processing on dynamic resources deployed using recipes.
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

// Run processes DELETE operations for dynamic resources deployed using recipes.
// It creates and delegates the request to DeleteResource controller to handle the deletion.
func (c *RecipeDeleteController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	deleteController, err := recipecontroller.NewDeleteResource(c.opts, &processor.DynamicProcessor{}, c.engine, c.configurationLoader)
	if err != nil {
		return ctrl.Result{}, err
	}

	return deleteController.Run(ctx, request)
}
