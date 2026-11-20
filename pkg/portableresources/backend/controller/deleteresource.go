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
	"slices"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/portableresources/processors"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/recipes/engine"
	"github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/resourceutil"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// SkippedRecipeDeploymentsProperty is the operation status property that lists the recipe deployments a forced
// delete skipped because their resource inventory was unknown. Resources they created may remain.
const SkippedRecipeDeploymentsProperty = "skippedRecipeDeployments"

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

// Run retrieves a resource from the database, parses the resource ID, gets the data model, deletes the output
// resources, and deletes the resource from the database. It returns an error if any of these steps fail.
func (c *DeleteResource[P, T]) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.DatabaseClient().Get(ctx, request.ResourceID)
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
	// A setup error skips recipe deletion only when no earlier deployment left cleanup state.
	pendingDeployments := data.ResourceMetadata().GetResourceStatus().PendingRecipeDeployments
	// unresolved tracks the pending deployments not yet recovered. After a successful forced delete,
	// these are the ones it skipped.
	unresolved := pendingDeployments
	if supportsRecipes && recipeDataModel.GetRecipe() != nil &&
		(recipeDataModel.GetRecipe().DeploymentStatus != util.RecipeSetupError || len(data.OutputResources()) > 0 || len(pendingDeployments) > 0) {
		resourceProperties, err := resourceutil.GetPropertiesFromResource(data)
		if err != nil {
			return ctrl.Result{}, err
		}
		recipeData := recipes.ResourceMetadata{
			Name:          recipeDataModel.GetRecipe().Name,
			EnvironmentID: data.ResourceMetadata().EnvironmentID(),
			ApplicationID: data.ResourceMetadata().ApplicationID(),
			Parameters:    recipeDataModel.GetRecipe().Parameters,
			ResourceID:    id.String(),
			Properties:    resourceProperties,
		}

		deleteOptions := engine.DeleteOptions{
			Recipe:             recipeData,
			OutputResources:    data.OutputResources(),
			PendingDeployments: pendingDeployments,
			Force:              request.Force,
		}
		if len(pendingDeployments) > 0 {
			deleteOptions.PersistCleanup = func(ctx context.Context, outputs []rpv1.OutputResource, pending []string) error {
				metadata := data.ResourceMetadata()
				status := metadata.GetResourceStatus()
				status.OutputResources = slices.Clone(outputs)
				status.PendingRecipeDeployments = slices.Clone(pending)
				metadata.SetResourceStatus(status)
				update := &database.Object{ID: request.ResourceID, Data: data}
				if err := c.DatabaseClient().Save(ctx, update, database.WithETag(obj.ETag)); err != nil {
					return err
				}
				obj.ETag = update.ETag
				unresolved = slices.Clone(pending)
				return nil
			}
		}
		err = c.engine.Delete(ctx, deleteOptions)
		if err != nil {
			if recipeError, ok := err.(*recipes.RecipeError); ok {
				return ctrl.NewFailedResult(recipeError.ErrorDetails), nil
			}
			return ctrl.Result{}, err
		}
	}

	// Load details about the runtime for the processor to access.
	runtimeConfiguration, err := c.loadRuntimeConfiguration(ctx, data.ResourceMetadata().EnvironmentID(), data.ResourceMetadata().ApplicationID(), data.GetBaseResource().ID)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.processor.Delete(ctx, data, processors.Options{
		RuntimeConfiguration: *runtimeConfiguration,
		Force:                request.Force,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.DatabaseClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	if len(unresolved) > 0 {
		// A forced delete skipped these deployments, so resources they created may remain. Report them to the caller.
		return ctrl.Result{Properties: map[string]any{SkippedRecipeDeploymentsProperty: unresolved}}, nil
	}
	return ctrl.Result{}, nil
}

func (c *DeleteResource[P, T]) loadRuntimeConfiguration(ctx context.Context, environmentID string, applicationID string, resourceID string) (*recipes.RuntimeConfiguration, error) {
	metadata := recipes.ResourceMetadata{EnvironmentID: environmentID, ApplicationID: applicationID, ResourceID: resourceID}
	config, err := c.configurationLoader.LoadConfiguration(ctx, metadata)
	if err != nil {
		return nil, err
	}

	return &config.Runtime, nil
}
