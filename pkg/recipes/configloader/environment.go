// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/rp/util"
)

var _ recipes.ConfigurationLoader = (*EnvironmentLoader)(nil)

const (
	Bicep = "bicep"
)

//go:generate mockgen -destination=./mock_config_loader.go -package=configloader -self_package github.com/project-radius/radius/pkg/recipes/configloader github.com/project-radius/radius/pkg/recipes ConfigurationLoader
type EnvironmentLoader struct {
	UCPClientOptions *arm.ClientOptions
}

// Load implements recipes.ConfigurationLoader. It fetches environment/application information and return runtime and provider configuration.
func (r *EnvironmentLoader) Load(ctx context.Context, recipe recipes.RecipeMetadata) (*recipes.Configuration, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, r.UCPClientOptions)
	if err != nil {
		return nil, err
	}

	var application *v20220315privatepreview.ApplicationResource
	if recipe.ApplicationID != "" {
		application, err = util.FetchApplication(ctx, recipe.ApplicationID, r.UCPClientOptions)
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
			configuration.Runtime.Kubernetes.Namespace, err = kube.FetchNamespaceFromEnvironmentResource(environment)
			if err != nil {
				return nil, err
			}
		}

	}

	if environment.Properties.Providers != nil && environment.Properties.Providers.Aws != nil {
		configuration.Providers.AWS.Scope = *environment.Properties.Providers.Aws.Scope
	}

	if environment.Properties.Providers != nil && environment.Properties.Providers.Azure != nil {
		configuration.Providers.Azure.Scope = *environment.Properties.Providers.Azure.Scope
	}

	return &configuration, nil
}

func (r *EnvironmentLoader) Lookup(ctx context.Context, recipe recipes.RecipeMetadata) (*recipes.RecipeDefinition, error) {
	environment, err := util.FetchEnvironment(ctx, recipe.EnvironmentID, r.UCPClientOptions)
	if err != nil {
		return nil, err
	}

	found, ok := environment.Properties.Recipes[recipe.Name]
	if !ok {
		return nil, &recipes.ErrRecipeNotFound{Name: recipe.Name, Environment: recipe.EnvironmentID}
	}

	return &recipes.RecipeDefinition{
		Driver:       Bicep,
		ResourceType: *found.LinkType,
		Parameters:   found.Parameters,
		TemplatePath: *found.TemplatePath,
	}, nil
}
