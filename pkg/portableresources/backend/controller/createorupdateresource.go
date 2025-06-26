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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
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
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// CreateOrUpdateResource is the async operation controller to create or update portable resources.
type CreateOrUpdateResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any] struct {
	ctrl.BaseController
	processor           processors.ResourceProcessor[P, T]
	engine              engine.Engine
	configurationLoader configloader.ConfigurationLoader
}

// NewCreateOrUpdateResource creates a new controller for creating or updating a resource with the given processor, engine,
// client, configurationLoader and options. The processor function will be called to process updates to the resource.
func NewCreateOrUpdateResource[P interface {
	*T
	rpv1.RadiusResourceModel
}, T any](opts ctrl.Options, processor processors.ResourceProcessor[P, T], eng engine.Engine, configurationLoader configloader.ConfigurationLoader) (ctrl.Controller, error) {
	return &CreateOrUpdateResource[P, T]{
		ctrl.NewBaseAsyncController(opts),
		processor,
		eng,
		configurationLoader,
	}, nil
}

// Run retrieves an existing resource, executes a recipe if needed, loads runtime configuration,
// processes the resource, cleans up any obsolete output resources, and saves the updated resource.
func (c *CreateOrUpdateResource[P, T]) Run(ctx context.Context, req *ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	storedResource, err := c.DatabaseClient().Get(ctx, req.ResourceID)
	if errors.Is(&database.ErrNotFound{ID: req.ResourceID}, err) {
		return ctrl.Result{}, err
	} else if err != nil {
		return ctrl.Result{}, err
	}

	resource := P(new(T))
	if err = storedResource.As(resource); err != nil {
		return ctrl.Result{}, err
	}

	// Clone existing output resources so we can diff them later.
	previousOutputResources := c.copyOutputResources(resource)

	// Load configuration
	metadata := recipes.ResourceMetadata{EnvironmentID: resource.ResourceMetadata().EnvironmentID(), ApplicationID: resource.ResourceMetadata().ApplicationID(), ResourceID: resource.GetBaseResource().ID}
	config, err := c.configurationLoader.LoadConfiguration(ctx, metadata)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Now we're ready to process recipes (if needed).
	recipeDataModel, supportsRecipes := any(resource).(datamodel.RecipeDataModel)
	var recipeOutput *recipes.RecipeOutput
	if supportsRecipes && recipeDataModel.GetRecipe() != nil {
		recipeOutput, err = c.executeRecipeIfNeeded(ctx, resource, recipeDataModel, previousOutputResources, config.Simulated)
		if err != nil {
			return c.handleRecipeError(ctx, err, recipeDataModel, req.ResourceID, storedResource.ETag, logger)
		}
	}

	if config.Simulated {
		logger.Info("The recipe was executed in simulation mode. No resources were deployed.")
	} else {
		// Now we're ready to process the resource. This will handle the updates to any user-visible state.
		err = c.processor.Process(ctx, resource, processors.Options{RecipeOutput: recipeOutput, RuntimeConfiguration: config.Runtime, UcpClient: c.BaseController.UcpClient()})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	if supportsRecipes {
		recipeData := recipeDataModel.GetRecipe()
		if recipeData != nil {
			recipeData.DeploymentStatus = util.Success
			recipeDataModel.SetRecipe(recipeData)
		}
		if recipeOutput != nil && recipeOutput.Status != nil {
			setRecipeStatus(resource, *recipeOutput.Status)
		}
	}

	update := &database.Object{
		Metadata: database.Metadata{
			ID: req.ResourceID,
		},
		Data: recipeDataModel.(rpv1.RadiusResourceModel),
	}
	err = c.DatabaseClient().Save(ctx, update, database.WithETag(storedResource.ETag))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

// handleRecipeError handles recipe execution errors: logs, updates status, persists, and returns the appropriate result.
func (c *CreateOrUpdateResource[P, T]) handleRecipeError(ctx context.Context, err error, recipeDataModel datamodel.RecipeDataModel, resourceID string, etag string, logger logr.Logger) (ctrl.Result, error) {
	var recipeErr *recipes.RecipeError
	if errors.As(err, &recipeErr) {
		logger.Error(recipeErr, fmt.Sprintf("failed to execute recipe. Encountered error while processing %s ", recipeErr.ErrorDetails.Target))

		// Set the deployment status to the recipe error code
		recipeDataModel.GetRecipe().DeploymentStatus = util.RecipeDeploymentStatus(recipeErr.DeploymentStatus)
		update := &database.Object{
			Metadata: database.Metadata{ID: resourceID},
			Data:     recipeDataModel.(rpv1.RadiusResourceModel),
		}

		// Save portable resource with updated deployment status to track errors during deletion.
		err = c.DatabaseClient().Save(ctx, update, database.WithETag(etag))
		if err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.NewFailedResult(recipeErr.ErrorDetails), nil
	}

	return ctrl.Result{}, err
}

func (c *CreateOrUpdateResource[P, T]) copyOutputResources(resource P) []string {
	previousOutputResources := []string{}
	for _, outputResource := range resource.OutputResources() {
		previousOutputResources = append(previousOutputResources, outputResource.ID.String())
	}
	return previousOutputResources
}

func (c *CreateOrUpdateResource[P, T]) executeRecipeIfNeeded(ctx context.Context, resource P, recipeDataModel datamodel.RecipeDataModel, prevState []string, simulated bool) (*recipes.RecipeOutput, error) {
	// Caller ensures recipeDataModel supports recipes and has a non-nil recipe
	recipe := recipeDataModel.GetRecipe()

	resourceProperties, err := resourceutil.GetPropertiesFromResource(resource)
	if err != nil {
		return nil, err
	}

	connectionsAndSourceIDs, err := resourceutil.GetConnectionNameandSourceIDs(resource)
	if err != nil {
		return nil, fmt.Errorf("failed to get connected resource IDs: %w", err)
	}
	connectedResourcesProperties := make(map[string]map[string]any)

	// If there are connected resources, we need to fetch their properties and add them to the recipe context.
	for connName, connectedResourceID := range connectionsAndSourceIDs {
		connectedResource, err := c.DatabaseClient().Get(ctx, connectedResourceID)
		if errors.Is(&database.ErrNotFound{ID: connectedResourceID}, err) {
			return nil, fmt.Errorf("connected resource %s not found: %w", connectedResourceID, err)
		} else if err != nil {
			return nil, fmt.Errorf("failed to get connected resource %s: %w", connectedResourceID, err)
		}

		connectedResourceProperties, err := resourceutil.GetPropertiesFromResource(connectedResource.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to get properties from connected resource %s: %w", connectedResourceID, err)
		}

		connectedResourcesProperties[connName] = connectedResourceProperties
	}

	metadata := recipes.ResourceMetadata{
		Name:                         recipe.Name,
		Parameters:                   recipe.Parameters,
		EnvironmentID:                resource.ResourceMetadata().EnvironmentID(),
		ApplicationID:                resource.ResourceMetadata().ApplicationID(),
		ResourceID:                   resource.GetBaseResource().ID,
		Properties:                   resourceProperties,
		ConnectedResourcesProperties: connectedResourcesProperties,
	}

	return c.engine.Execute(ctx, engine.ExecuteOptions{
		BaseOptions: engine.BaseOptions{
			Recipe: metadata,
		},
		PreviousState: prevState,
		Simulated:     simulated,
	})
}

// setRecipeStatus sets the recipe status for the given resource model.
// It retrieves the resource metadata from the provided model, deep copies the current resource status,
// updates the Recipe field with the supplied recipeStatus, and then applies the updated status back to the resource.
func setRecipeStatus[P rpv1.RadiusResourceModel](data P, recipeStatus rpv1.RecipeStatus) {
	rm := data.ResourceMetadata()
	status := rm.GetResourceStatus().DeepCopyRecipeStatus()
	status.Recipe = &recipeStatus
	rm.SetResourceStatus(status)
}
