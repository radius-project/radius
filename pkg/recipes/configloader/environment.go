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

package configloader

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/rp/kube"
	"github.com/radius-project/radius/pkg/rp/util"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var (
	ErrUnsupportedComputeKind = errors.New("unsupported compute kind in environment resource")
)

//go:generate mockgen -typed -destination=./mock_config_loader.go -package=configloader -self_package github.com/radius-project/radius/pkg/recipes/configloader github.com/radius-project/radius/pkg/recipes/configloader ConfigurationLoader

var _ ConfigurationLoader = (*environmentLoader)(nil)

// NewEnvironmentLoader creates a new environmentLoader instance with the given ARM Client Options.
func NewEnvironmentLoader(armOptions *arm.ClientOptions) ConfigurationLoader {
	return &environmentLoader{ArmClientOptions: armOptions}
}

// EnvironmentLoader struct is initialized with arm clients and provides functionality to get environment configuration and recipe information.
type environmentLoader struct {
	// ArmClientOptions represents the client options for ARM clients.
	ArmClientOptions *arm.ClientOptions
}

// LoadConfiguration fetches an environment and an application (if provided) and returns a configuration based on them. It returns
// an error if either the environment or the application (if provided) cannot be fetched.
func (e *environmentLoader) LoadConfiguration(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.Configuration, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
	if err != nil {
		return nil, err
	}

	var application *v20231001preview.ApplicationResource
	if recipe.ApplicationID != "" {
		application, err = util.FetchApplication(ctx, recipe.ApplicationID, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}
	}

	return getConfiguration(environment, application)
}

func getConfiguration(environment *v20231001preview.EnvironmentResource, application *v20231001preview.ApplicationResource) (*recipes.Configuration, error) {
	config := recipes.Configuration{
		Runtime:      recipes.RuntimeConfiguration{},
		Providers:    datamodel.Providers{},
		RecipeConfig: datamodel.RecipeConfigProperties{},
	}

	switch environment.Properties.Compute.(type) {
	case *v20231001preview.KubernetesCompute:
		config.Runtime.Kubernetes = &recipes.KubernetesRuntime{}
		var err error

		// Environment-scoped namespace must be given all the time.
		config.Runtime.Kubernetes.EnvironmentNamespace, err = kube.FetchNamespaceFromEnvironmentResource(environment)
		if err != nil {
			return nil, err
		}

		if application != nil {
			config.Runtime.Kubernetes.Namespace, err = kube.FetchNamespaceFromApplicationResource(application)
			if err != nil {
				return nil, err
			}
		} else {
			// Use environment-scoped namespace if application is not set.
			config.Runtime.Kubernetes.Namespace = config.Runtime.Kubernetes.EnvironmentNamespace
		}
	case *v20231001preview.AzureContainerInstanceCompute:
		config.Runtime.AzureContainerInstances = &recipes.AzureContainerInstancesRuntime{}
	default:
		return nil, ErrUnsupportedComputeKind
	}

	// convert versioned Environment resource to internal datamodel.
	env, err := environment.ConvertTo()
	if err != nil {
		return nil, err
	}

	envDatamodel := env.(*datamodel.Environment)
	if environment.Properties.Providers != nil {
		config.Providers = envDatamodel.Properties.Providers
	}

	if environment.Properties.RecipeConfig != nil {
		config.RecipeConfig = envDatamodel.Properties.RecipeConfig
	}

	if environment.Properties.Simulated != nil && *environment.Properties.Simulated {
		config.Simulated = true
	}

	return &config, nil
}

// LoadRecipe fetches the recipe information from the environment. It returns an error if the environment cannot be fetched.
func (e *environmentLoader) LoadRecipe(ctx context.Context, recipe *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
	if err != nil {
		return nil, err
	}

	envDefinition, err := getRecipeDefinition(ctx, environment, recipe, e.ArmClientOptions)
	if err != nil {
		return nil, err
	}

	return envDefinition, err
}

func getRecipeDefinition(ctx context.Context, environment *v20231001preview.EnvironmentResource, recipe *recipes.ResourceMetadata, armOptions *arm.ClientOptions) (*recipes.EnvironmentDefinition, error) {
	resource, err := resources.ParseResource(recipe.ResourceID)
	if err != nil {
		err := fmt.Errorf("failed to parse resourceID: %q %w", recipe.ResourceID, err)
		return nil, recipes.NewRecipeError(recipes.RecipeValidationFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	// First try to find recipe in environment's recipe packs
	env, err := environment.ConvertTo()
	if err != nil {
		return nil, err
	}
	envDatamodel := env.(*datamodel.Environment)

	if envDatamodel.Properties.RecipePacks != nil {
		recipePackDefinition, err := fetchRecipePacks(ctx, envDatamodel.Properties.RecipePacks, armOptions, resource.Type())
		if err == nil {
			// Found recipe in recipe pack
			definition := &recipes.EnvironmentDefinition{
				Name:         "default",
				Driver:       recipePackDefinition.RecipeKind,
				ResourceType: resource.Type(),
				Parameters:   recipePackDefinition.Parameters,
				TemplatePath: recipePackDefinition.RecipeLocation,
			}
			return definition, nil
		}
	}

	// Fall back to environment.Properties.Recipes if recipe pack not found
	if environment.Properties.Recipes == nil {
		err := fmt.Errorf("could not find recipe %q in environment %q", recipe.Name, recipe.EnvironmentID)
		return nil, recipes.NewRecipeError(recipes.RecipeNotFoundFailure, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	recipeName := "default"
	found, ok := environment.Properties.Recipes[resource.Type()][recipeName]
	if !ok {
		err := fmt.Errorf("could not find recipe %q in environment %q", recipe.Name, recipe.EnvironmentID)
		return nil, recipes.NewRecipeError(recipes.RecipeNotFoundFailure, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	definition := &recipes.EnvironmentDefinition{
		Name:         recipeName,
		Driver:       *found.GetRecipeProperties().TemplateKind,
		ResourceType: resource.Type(),
		Parameters:   found.GetRecipeProperties().Parameters,
		TemplatePath: *found.GetRecipeProperties().TemplatePath,
	}
	switch c := found.(type) {
	case *v20231001preview.TerraformRecipeProperties:
		definition.TemplateVersion = *c.TemplateVersion
	case *v20231001preview.BicepRecipeProperties:
		if c.PlainHTTP != nil {
			definition.PlainHTTP = *c.PlainHTTP
		}
	}

	return definition, nil
}

// fetchRecipePacks fetches recipe pack resources from the given recipe pack IDs and returns the first recipe pack that has a recipe for the specified resource type.
func fetchRecipePacks(ctx context.Context, recipePackIDs []string, armOptions *arm.ClientOptions, resourceType string) (*recipes.RecipePackDefinition, error) {
	if recipePackIDs == nil {
		return nil, fmt.Errorf("no recipe packs configured")
	}

	if resourceType == "" {
		return nil, fmt.Errorf("resource type cannot be empty")
	}

	for _, recipePackID := range recipePackIDs {
		recipePackResource, err := FetchRecipePack(ctx, recipePackID, armOptions)
		if err != nil {
			return nil, err
		}

		// Convert recipes map
		for recipePackResourceType, definition := range recipePackResource.Properties.Recipes {
			if strings.EqualFold(recipePackResourceType, resourceType) {
				return &recipes.RecipePackDefinition{
					RecipeKind:     string(*definition.RecipeKind),
					RecipeLocation: string(*definition.RecipeLocation),
					Parameters:     definition.Parameters,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no recipe pack found with recipe for resource type %q", resourceType)
}
