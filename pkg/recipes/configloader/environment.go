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
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	recipes_util "github.com/radius-project/radius/pkg/recipes/util"
	"github.com/radius-project/radius/pkg/rp/kube"
	"github.com/radius-project/radius/pkg/rp/util"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/resources/radius"
)

var (
	ErrUnsupportedComputeKind = errors.New("unsupported compute kind in environment resource")
	ErrBadEnvID               = errors.New("could not parse environment ID")
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
	envID, err := resources.Parse(recipe.EnvironmentID)
	if err != nil {
		return nil, ErrBadEnvID
	}

	var environment *v20231001preview.EnvironmentResource
	if strings.EqualFold(envID.ProviderNamespace(), radius.NamespaceApplicationsCore) {
		environment, err = util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
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
	} else {
		envV20250801, err := util.FetchEnvironmentV20250801(ctx, recipe.EnvironmentID, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}

		return getConfigurationV20250801(envV20250801)
	}

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

func getConfigurationV20250801(environment *v20250801preview.EnvironmentResource) (*recipes.Configuration, error) {
	config := recipes.Configuration{
		Runtime:      recipes.RuntimeConfiguration{},
		Providers:    datamodel.Providers{},
		RecipeConfig: datamodel.RecipeConfigProperties{},
	}

	config.Runtime.Kubernetes = &recipes.KubernetesRuntime{}
	var err error

	env, err := environment.ConvertTo()
	if err != nil {
		return nil, err
	}

	envDatamodel := env.(*datamodel.Environment_v20250801preview)
	if envDatamodel.Properties.Providers != nil {
		if envDatamodel.Properties.Providers.Azure != nil {
			config.Providers.Azure = datamodel.ProvidersAzure{
				Scope: envDatamodel.Properties.Providers.Azure.SubscriptionId,
			}
		}
		if envDatamodel.Properties.Providers.AWS != nil {
			config.Providers.AWS = datamodel.ProvidersAWS{
				Scope: envDatamodel.Properties.Providers.AWS.Scope,
			}
		}
	}
	// Radius enables keying in of a preexisting namespace for kubernetes resources using
	// properties.providers.kubernetes.namespace. However, it does not mandate configuring a
	// Kubernetes provider since the recipe can have the namespace details and deploy resources successfully.
	// We should converge EnvironmentNamespace and Namespace once we remove Applications.Core support, since
	// We no longer have Application Namespaces.
	config.Runtime.Kubernetes.EnvironmentNamespace = kube.FetchNamespaceFromEnvironmentResourceV20250801(environment)
	config.Runtime.Kubernetes.Namespace = config.Runtime.Kubernetes.EnvironmentNamespace

	if envDatamodel.Properties.Simulated {
		config.Simulated = true
	}

	return &config, nil
}

// LoadRecipe fetches the recipe information from the environment. It returns an error if the environment cannot be fetched.
func (e *environmentLoader) LoadRecipe(ctx context.Context, recipe *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
	envID, err := resources.Parse(recipe.EnvironmentID)
	if err != nil {
		return nil, ErrBadEnvID
	}
	var envDefinition *recipes.EnvironmentDefinition
	if strings.EqualFold(envID.ProviderNamespace(), radius.NamespaceApplicationsCore) {
		environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}
		envDefinition, err = getRecipeDefinition(environment, recipe)
		if err != nil {
			return nil, err
		}

	} else {
		environment, err := util.FetchEnvironmentV20250801(ctx, recipe.EnvironmentID, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}
		envDefinition, err = getRecipeDefinitionFromEnvironmentV20250801(ctx, environment, recipe, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}

	}
	return envDefinition, err
}

func getRecipeDefinition(environment *v20231001preview.EnvironmentResource, recipe *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
	if environment.Properties.Recipes == nil {
		err := fmt.Errorf("could not find recipe %q in environment %q", recipe.Name, recipe.EnvironmentID)
		return nil, recipes.NewRecipeError(recipes.RecipeNotFoundFailure, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}

	resource, err := resources.ParseResource(recipe.ResourceID)
	if err != nil {
		err := fmt.Errorf("failed to parse resourceID: %q %w", recipe.ResourceID, err)
		return nil, recipes.NewRecipeError(recipes.RecipeValidationFailed, err.Error(), recipes_util.RecipeSetupError, recipes.GetErrorDetails(err))
	}
	recipeName := recipe.Name
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

func getRecipeDefinitionFromEnvironmentV20250801(ctx context.Context, environment *v20250801preview.EnvironmentResource, recipe *recipes.ResourceMetadata, armOptions *arm.ClientOptions) (*recipes.EnvironmentDefinition, error) {
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
	envDatamodel := env.(*datamodel.Environment_v20250801preview)

	if envDatamodel.Properties.RecipePacks != nil {
		recipeDefinition, err := fetchRecipeDefinition(ctx, envDatamodel.Properties.RecipePacks, armOptions, resource.Type())
		if err != nil {
			return nil, err
		}

		// Reconcile parameters from recipe pack and environment-level recipe parameters
		parameters := reconcileRecipeParameters(recipeDefinition.Parameters, envDatamodel.Properties.RecipeParameters, resource.Type())

		// TODO: For now, we can set "Name" to default as recipe packs don't have named recipes.
		// We will remove this field from EnvironmentDefinition once we deprecate Applications.Core.
		definition := &recipes.EnvironmentDefinition{
			Name:         "default",
			Driver:       recipeDefinition.RecipeKind,
			ResourceType: resource.Type(),
			Parameters:   parameters,
			TemplatePath: recipeDefinition.RecipeLocation,
		}
		return definition, nil
	}

	return nil, fmt.Errorf("could not find any recipe pack for %q in environment %q", resource.Type(), recipe.EnvironmentID)
}

// fetchRecipeDefinition fetches recipe pack resources from the given recipe pack IDs and returns
// the recipe definition from the first recipe pack that has a recipe defined for the specified resource type.
// There cannot be more than one recipe pack with a recipe definition for the same resource type as part of an environment.
func fetchRecipeDefinition(ctx context.Context, recipePackIDs []string, armOptions *arm.ClientOptions, resourceType string) (*recipes.RecipeDefinition, error) {
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
				return &recipes.RecipeDefinition{
					RecipeKind:     string(*definition.RecipeKind),
					RecipeLocation: string(*definition.RecipeLocation),
					Parameters:     definition.Parameters,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("no recipe pack found with recipe for resource type %q", resourceType)
}

// reconcileRecipeParameters merges recipe pack parameters with environment-level recipe parameters.
// Environment-level parameters override recipe pack parameters when the same key exists.
func reconcileRecipeParameters(recipePackParams map[string]any, envRecipeParams map[string]map[string]any, resourceType string) map[string]any {
	parameters := make(map[string]any)

	// Start with recipe pack parameters
	for k, v := range recipePackParams {
		parameters[k] = v
	}

	// Override with environment-level recipe parameters for this resource type
	if envRecipeParams != nil {
		if params, ok := envRecipeParams[resourceType]; ok {
			for k, v := range params {
				parameters[k] = v
			}
		}
	}

	return parameters
}
