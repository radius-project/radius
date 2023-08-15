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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/rp/util"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var (
	ErrUnsupportedComputeKind = errors.New("unsupported compute kind in environment resource")
)

//go:generate mockgen -destination=./mock_config_loader.go -package=configloader -self_package github.com/project-radius/radius/pkg/recipes/configloader github.com/project-radius/radius/pkg/recipes/configloader ConfigurationLoader

var _ ConfigurationLoader = (*environmentLoader)(nil)

// # Function Explanation
//
// NewEnvironmentLoader creates a new environmentLoader instance with the given ARM Client Options.
func NewEnvironmentLoader(armOptions *arm.ClientOptions) ConfigurationLoader {
	return &environmentLoader{ArmClientOptions: armOptions}
}

// EnvironmentLoader struct is initialized with arm clients and provides functionality to get environment configuration and recipe information.
type environmentLoader struct {
	// ArmClientOptions represents the client options for ARM clients.
	ArmClientOptions *arm.ClientOptions
}

// # Function Explanation
//
// LoadConfiguration fetches an environment and an application (if provided) and returns a configuration based on them. It returns
// an error if either the environment or the application (if provided) cannot be fetched.
func (e *environmentLoader) LoadConfiguration(ctx context.Context, recipe recipes.ResourceMetadata) (*recipes.Configuration, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
	if err != nil {
		return nil, err
	}

	var application *v20220315privatepreview.ApplicationResource
	if recipe.ApplicationID != "" {
		application, err = util.FetchApplication(ctx, recipe.ApplicationID, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}
	}

	return getConfiguration(environment, application)
}

func getConfiguration(environment *v20220315privatepreview.EnvironmentResource, application *v20220315privatepreview.ApplicationResource) (*recipes.Configuration, error) {
	config := recipes.Configuration{
		Runtime:   recipes.RuntimeConfiguration{},
		Providers: datamodel.Providers{},
	}

	switch environment.Properties.Compute.(type) {
	case *v20220315privatepreview.KubernetesCompute:
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

	default:
		return nil, ErrUnsupportedComputeKind
	}

	providers := environment.Properties.Providers
	if providers != nil {
		if providers.Aws != nil {
			config.Providers.AWS.Scope = to.String(providers.Aws.Scope)
		}
		if providers.Azure != nil {
			config.Providers.Azure.Scope = to.String(providers.Azure.Scope)
		}
	}

	return &config, nil
}

// # Function Explanation
//
// LoadRecipe fetches the recipe information from the environment. It returns an error if the environment cannot be fetched.
func (e *environmentLoader) LoadRecipe(ctx context.Context, recipe *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
	if err != nil {
		return nil, err
	}
	return getRecipeDefinition(environment, recipe)
}

func getRecipeDefinition(environment *v20220315privatepreview.EnvironmentResource, recipe *recipes.ResourceMetadata) (*recipes.EnvironmentDefinition, error) {
	if environment.Properties.Recipes == nil {
		return nil, &recipes.ErrRecipeNotFound{Name: recipe.Name, Environment: recipe.EnvironmentID}
	}

	resource, err := resources.ParseResource(recipe.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resourceID: %q %w", recipe.ResourceID, err)
	}
	recipeName := recipe.Name
	found, ok := environment.Properties.Recipes[resource.Type()][recipeName]
	if !ok {
		return nil, &recipes.ErrRecipeNotFound{Name: recipe.Name, Environment: recipe.EnvironmentID}
	}

	definition := &recipes.EnvironmentDefinition{
		Name:         recipeName,
		Driver:       *found.GetEnvironmentRecipeProperties().TemplateKind,
		ResourceType: resource.Type(),
		Parameters:   found.GetEnvironmentRecipeProperties().Parameters,
		TemplatePath: *found.GetEnvironmentRecipeProperties().TemplatePath,
	}
	switch c := found.(type) {
	case *v20220315privatepreview.TerraformRecipeProperties:
		definition.TemplateVersion = *c.TemplateVersion
	}

	return definition, nil
}
