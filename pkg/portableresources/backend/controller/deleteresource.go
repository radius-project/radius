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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// DeleteResource is the async operation controller to delete a portable resource.
type DeleteResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] struct {
	ctrl.BaseController
	processor           processors.ResourceProcessor[P, T]
	engine              engine.Engine
	configurationLoader configloader.ConfigurationLoader
}

// NewDeleteResource creates a new DeleteResource controller which is used to delete resources asynchronously.
func NewDeleteResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](opts ctrl.Options, processor processors.ResourceProcessor[P, T], eng engine.Engine, configurationLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &DeleteResource[P, T]{
		ctrl.NewBaseAsyncController(opts),
		processor,
		eng,
		configurationLoader,
	}, nil
}

// Run retrieves a resource from storage, parses the resource ID, gets the data model, deletes the output
// resources, and deletes the resource from storage. It returns an error if any of these steps fail.
func (c *DeleteResource[P, T]) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	data := P(new(T))
	if err = obj.As(data); err != nil {
		return ctrl.Result{}, err
	}

	recipeDataModel, supportsRecipes := any(data).(datamodel.RecipeDataModel)
	if supportsRecipes && recipeDataModel.Recipe() != nil {
		recipeData := recipes.ResourceMetadata{
			Name:          recipeDataModel.Recipe().Name,
			EnvironmentID: data.ResourceMetadata().Environment,
			ApplicationID: data.ResourceMetadata().Application,
			Parameters:    recipeDataModel.Recipe().Parameters,
			ResourceID:    id.String(),
		}

		err = c.engine.Delete(ctx, recipeData, data.OutputResources())
		if err != nil {
			if recipeError, ok := err.(*recipes.RecipeError); ok {
				return ctrl.NewFailedResult(recipeError.ErrorDetails), nil
			}
			return ctrl.Result{}, err
		}
	}

	// Load details about the runtime for the processor to access.
	runtimeConfiguration, err := c.loadRuntimeConfiguration(ctx, data.ResourceMetadata().Environment, data.ResourceMetadata().Application, data.GetBaseResource().ID)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.processor.Delete(ctx, data, processors.Options{
		RuntimeConfiguration: *runtimeConfiguration,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.StorageClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func (c *DeleteResource[P, T]) loadRuntimeConfiguration(ctx context.Context, environmentID string, applicationID string, resourceID string) (*recipes.RuntimeConfiguration, error) {
	metadata := recipes.ResourceMetadata{EnvironmentID: environmentID, ApplicationID: applicationID, ResourceID: resourceID}
	config, err := c.configurationLoader.LoadConfiguration(ctx, metadata)
	if err != nil {
		return nil, err
	}

	return &config.Runtime, nil
}
