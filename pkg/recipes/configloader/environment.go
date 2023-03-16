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

var _ ConfigurationLoader = (*EnvironmentLoader)(nil)

const (
	Bicep = "bicep"
)

type EnvironmentLoader struct {
	UCPClientOptions *arm.ClientOptions
}

// Load implements recipes.ConfigurationLoader. It fetches environment/application information and return runtime and provider configuration.
func (r *EnvironmentLoader) Load(ctx context.Context, recipe recipes.RecipeContext) (*Configuration, error) {
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

func getConfiguration(environment *v20220315privatepreview.EnvironmentResource, application *v20220315privatepreview.ApplicationResource) (*Configuration, error) {
	configuration := Configuration{Runtime: RuntimeConfiguration{}, Providers: datamodel.Providers{}}
	if environment.Properties.Compute != nil && *environment.Properties.Compute.GetEnvironmentCompute().Kind == v20220315privatepreview.EnvironmentComputeKindKubernetes {
		// This is a Kubernetes environment
		configuration.Runtime.Kubernetes = &KubernetesRuntime{}
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
