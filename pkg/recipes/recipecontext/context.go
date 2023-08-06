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

	coredm "github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/recipes"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

var (
	ErrParseFormat = "failed to parse %s: %q while building the recipe context parameter %w"
)

// New creates the context parameter for the recipe with the link, environment and application info
func New(metadata *recipes.ResourceMetadata, config *recipes.Configuration) (*Context, error) {
	parsedLink, err := resources.ParseResource(metadata.ResourceID)
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
				Name: parsedLink.Name(),
				ID:   metadata.ResourceID,
			},
			Type: parsedLink.Type(),
		},
		Environment: ResourceInfo{
			Name: parsedEnv.Name(),
			ID:   metadata.EnvironmentID,
		},
		Runtime: recipes.RuntimeConfiguration{
			Kubernetes: &recipes.KubernetesRuntime{
				Namespace:            config.Runtime.Kubernetes.Namespace,
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
		subID := p.FindScope(resources.SubscriptionsSegment)
		rgName := p.FindScope(resources.ResourceGroupsSegment)
		recipeContext.Azure = ProviderAzure{
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
		recipeContext.AWS = ProviderAWS{
			Region:  p.FindScope(resources.RegionsSegment),
			Account: p.FindScope(resources.AccountsSegment),
		}
	}

	return &recipeContext, nil
}
