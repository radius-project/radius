// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package configloader

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/rp/util"
)

var _ recipes.ConfigurationLoader = (*EnvironmentLoader)(nil)

const (
	Bicep = "bicep"
)

type EnvironmentLoader struct {
	UCPClientOptions *arm.ClientOptions
}

// Load implements recipes.ConfigurationLoader
func (r *EnvironmentLoader) Load(ctx context.Context, recipe recipes.Recipe) (*recipes.Configuration, error) {
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

	configuration := recipes.Configuration{Runtime: recipes.RuntimeConfiguration{}, Providers: map[string]map[string]any{}}
	if *environment.Properties.Compute.GetEnvironmentCompute().Kind == v20220315privatepreview.EnvironmentComputeKindKubernetes {
		// This is a Kubernetes environment
		configuration.Runtime.Kubernetes = &recipes.KubernetesRuntime{}

		// Prefer application namespace if set
		if application != nil {
			configuration.Runtime.Kubernetes.Namespace, err = kube.FetchNameSpaceFromApplicationResource(application)
			if err != nil {
				return nil, err
			}
		} else {
			configuration.Runtime.Kubernetes.Namespace, err = kube.FetchNameSpaceFromEnvironmentResource(environment)
			if err != nil {
				return nil, err
			}
		}

	}

	if environment.Properties.Providers != nil && environment.Properties.Providers.Aws != nil {
		configuration.Providers[resourcemodel.ProviderAWS] = map[string]any{
			"scope": *environment.Properties.Providers.Aws.Scope,
		}
	}

	if environment.Properties.Providers != nil && environment.Properties.Providers.Azure != nil {
		configuration.Providers[resourcemodel.ProviderAzure] = map[string]any{
			"scope": *environment.Properties.Providers.Azure.Scope,
		}
	}

	return &configuration, nil
}
