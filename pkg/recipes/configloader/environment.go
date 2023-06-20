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
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/rp/util"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//go:generate mockgen -destination=./mock_config_loader.go -package=configloader -self_package github.com/project-radius/radius/pkg/recipes/configloader github.com/project-radius/radius/pkg/recipes/configloader ConfigurationLoader

var _ ConfigurationLoader = (*environmentLoader)(nil)

func NewEnvironmentLoader(armOptions *arm.ClientOptions) ConfigurationLoader {
	return &environmentLoader{ArmClientOptions: armOptions}
}

// EnvironmentLoader struct is initialized with arm clients and provides functionality to get environment configuration and recipe information.
type environmentLoader struct {
	// ArmClientOptions represents the client options for ARM clients.
	ArmClientOptions *arm.ClientOptions
}

// LoadConfiguration fetches environment/application information and return runtime and provider configuration.
func (e *environmentLoader) LoadConfiguration(ctx context.Context, recipe recipes.Metadata) (*recipes.Configuration, error) {
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
	configuration := recipes.Configuration{Runtime: recipes.RuntimeConfiguration{}, Providers: datamodel.Providers{}}
	if environment.Properties.Compute != nil && *environment.Properties.Compute.GetEnvironmentCompute().Kind == v20220315privatepreview.EnvironmentComputeKindKubernetes {
		// This is a Kubernetes environment
		configuration.Runtime.Kubernetes = &recipes.KubernetesRuntime{}
		var err error
		// Prefer application namespace if set
		if application != nil {
			configuration.Runtime.Kubernetes.Namespace, err = kube.FetchNamespaceFromApplicationResource(application)
			if err != nil {
				return nil, err
			}
		} else {
			configuration.Runtime.Kubernetes.EnvironmentNamespace, err = kube.FetchNamespaceFromEnvironmentResource(environment)
			if err != nil {
				return nil, err
			}
		}

	}

	if environment.Properties.Providers != nil {
		if environment.Properties.Providers.Aws != nil {
			configuration.Providers.AWS.Scope = *environment.Properties.Providers.Aws.Scope
		}

		if environment.Properties.Providers.Azure != nil {
			configuration.Providers.Azure.Scope = *environment.Properties.Providers.Azure.Scope
		}
	}

	return &configuration, nil
}

// LoadRecipe fetches the recipe information from the environment.
func (e *environmentLoader) LoadRecipe(ctx context.Context, recipe *recipes.Metadata) (*recipes.Definition, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, e.ArmClientOptions)
	if err != nil {
		return nil, err
	}
	return getRecipeDefinition(environment, recipe)
}

func getRecipeDefinition(environment *v20220315privatepreview.EnvironmentResource, recipe *recipes.Metadata) (*recipes.Definition, error) {
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

	return &recipes.Definition{
		Name:         recipeName,
		Driver:       *found.TemplateKind,
		ResourceType: resource.Type(),
		Parameters:   found.Parameters,
		TemplatePath: *found.TemplatePath,
	}, nil
}
