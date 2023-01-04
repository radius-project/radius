// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package environment

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var _ recipes.ConfigurationLoader = (*EnvironmentLoader)(nil)
var _ recipes.Repository = (*EnvironmentLoader)(nil)

type EnvironmentLoader struct {
	UCPClientOptions *arm.ClientOptions
}

// Load implements recipes.ConfigurationLoader
func (r *EnvironmentLoader) Load(ctx context.Context, recipe recipes.Recipe) (*recipes.Configuration, error) {
	environment, err := r.fetchEnvironment(ctx, recipe)
	if err != nil {
		return nil, err
	}

	var application *v20220315privatepreview.ApplicationResource
	if recipe.ApplicationID != "" {
		application, err = r.fetchApplication(ctx, recipe)
		if err != nil {
			return nil, err
		}
	}

	configuration := recipes.Configuration{Runtime: recipes.RuntimeConfiguration{}, Providers: map[string]map[string]interface{}{}}
	if *environment.Properties.Compute.GetEnvironmentCompute().Kind == v20220315privatepreview.EnvironmentComputeKindKubernetes {
		// This is a Kubernetes environment
		configuration.Runtime.Kubernetes = &recipes.KubernetesRuntime{}

		kubernetes := environment.Properties.Compute.(*v20220315privatepreview.KubernetesCompute)
		configuration.Runtime.Kubernetes.Namespace = *kubernetes.Namespace

		// Prefer application namespace if set
		if application != nil {
			kubernetes := application.Properties.Status.Compute.(*v20220315privatepreview.KubernetesCompute)
			configuration.Runtime.Kubernetes.Namespace = *kubernetes.Namespace
		}
	}

	if environment.Properties.Providers != nil && environment.Properties.Providers.Azure != nil {
		configuration.Providers["azure"] = map[string]interface{}{
			"scope": *environment.Properties.Providers.Azure.Scope,
		}
	}

	return &configuration, nil
}

// Lookup implements recipes.Repository
func (r *EnvironmentLoader) Lookup(ctx context.Context, recipe recipes.Recipe) (*recipes.Definition, error) {
	environment, err := r.fetchEnvironment(ctx, recipe)
	if err != nil {
		return nil, err
	}

	found, ok := environment.Properties.Recipes[recipe.Name]
	if !ok {
		return nil, &recipes.ErrRecipeNotFound{Name: recipe.Name, Environment: recipe.EnvironmentID}
	}

	return &recipes.Definition{
		Driver:       "bicep",
		ResourceType: *found.LinkType,
		Parameters:   found.Parameters,
		TemplatePath: *found.TemplatePath,
	}, nil
}

func (r *EnvironmentLoader) fetchApplication(ctx context.Context, recipe recipes.Recipe) (*v20220315privatepreview.ApplicationResource, error) {
	applicationID, err := resources.ParseResource(recipe.ApplicationID)
	if err != nil {
		return nil, err
	}

	client, err := v20220315privatepreview.NewApplicationsClient(applicationID.RootScope(), &aztoken.AnonymousCredential{}, r.UCPClientOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, applicationID.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.ApplicationResource, nil
}

func (r *EnvironmentLoader) fetchEnvironment(ctx context.Context, recipe recipes.Recipe) (*v20220315privatepreview.EnvironmentResource, error) {
	environmentID, err := resources.ParseResource(recipe.EnvironmentID)
	if err != nil {
		return nil, err
	}

	client, err := v20220315privatepreview.NewEnvironmentsClient(environmentID.RootScope(), &aztoken.AnonymousCredential{}, r.UCPClientOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, environmentID.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.EnvironmentResource, nil
}
