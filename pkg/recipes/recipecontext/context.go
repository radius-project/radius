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

package recipecontext

import (
	"fmt"

	coredm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
	resources_azure "github.com/radius-project/radius/pkg/ucp/resources/azure"
)

var (
	ErrParseFormat = "failed to parse %s: %q while building the recipe context parameter %w"
)

// New creates the context parameter for the recipe with the portable resource, environment, and application info
func New(metadata *recipes.ResourceMetadata, config *recipes.Configuration) (*Context, error) {
	parsedResource, err := resources.ParseResource(metadata.ResourceID)
	if err != nil {
		return nil, fmt.Errorf(ErrParseFormat, "resourceID", metadata.ResourceID, err)
	}

	parsedEnv, err := resources.ParseResource(metadata.EnvironmentID)
	if err != nil {
		return nil, fmt.Errorf(ErrParseFormat, "environmentID", metadata.EnvironmentID, err)
	}

	recipeContext := Context{
		Resource: Resource{
			ResourceInfo: ResourceInfo{
				Name: parsedResource.Name(),
				ID:   metadata.ResourceID,
			},
			Type: parsedResource.Type(),
		},
		Environment: ResourceInfo{
			Name: parsedEnv.Name(),
			ID:   metadata.EnvironmentID,
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            config.Runtime.Kubernetes.EnvironmentNamespace,
				EnvironmentNamespace: config.Runtime.Kubernetes.EnvironmentNamespace,
			},
		},
	}

	if metadata.ApplicationID != "" {
		parsedApp, err := resources.ParseResource(metadata.ApplicationID)
		if err != nil {
			return nil, fmt.Errorf(ErrParseFormat, "applicationID", metadata.ApplicationID, err)
		}
		recipeContext.Application.ID = metadata.ApplicationID
		recipeContext.Application.Name = parsedApp.Name()
		recipeContext.Runtime.Kubernetes.Namespace = config.Runtime.Kubernetes.Namespace
	}

	providers := config.Providers

	if providers.Azure != (coredm.ProvidersAzure{}) {
		p, err := resources.ParseScope(providers.Azure.Scope)
		if err != nil {
			return nil, fmt.Errorf(ErrParseFormat, "Azure scope", providers.Azure.Scope, err)
		}

		subID := p.FindScope(resources_azure.ScopeSubscriptions)
		rgName := p.FindScope(resources_azure.ScopeResourceGroups)
		recipeContext.Azure = &ProviderAzure{
			ResourceGroup: AzureResourceGroup{
				Name: rgName,
				ID:   "/subscriptions/" + subID + "/resourceGroups/" + rgName,
			},
			Subscription: AzureSubscription{
				SubscriptionID: subID,
				ID:             "/subscriptions/" + subID,
			},
		}
	}

	if providers.AWS != (coredm.ProvidersAWS{}) {
		p, err := resources.ParseScope(providers.AWS.Scope)
		if err != nil {
			return nil, fmt.Errorf(ErrParseFormat, "AWS scope", providers.AWS.Scope, err)
		}
		recipeContext.AWS = &ProviderAWS{
			Region:  p.FindScope(resources_aws.ScopeRegions),
			Account: p.FindScope(resources_aws.ScopeAccounts),
		}
	}

	return &recipeContext, nil
}
