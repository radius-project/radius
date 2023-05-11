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

package cmd

import (
	"context"
	"fmt"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/aws"
	"github.com/project-radius/radius/pkg/cli/azure"
	"github.com/project-radius/radius/pkg/cli/clients"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
)

// CreateEnvProviders forms the provider scope from the given
func CreateEnvProviders(providersList []any) (corerp.Providers, error) {
	var res corerp.Providers
	for _, provider := range providersList {
		switch p := provider.(type) {
		case *azure.Provider:
			if res.Azure != nil {
				return res, &cli.FriendlyError{Message: "Only one azure provider can be configured to a scope"}
			}
			if p == nil {
				break
			}
			res.Azure = &corerp.ProvidersAzure{
				Scope: to.Ptr("/subscriptions/" + p.SubscriptionID + "/resourceGroups/" + p.ResourceGroup),
			}
		case *aws.Provider:
			if res.Aws != nil {
				return res, &cli.FriendlyError{Message: "Only one aws provider can be configured to a scope"}
			}
			if p == nil {
				break
			}
			res.Aws = &corerp.ProvidersAws{
				Scope: to.Ptr("/planes/aws/aws/accounts/" + p.AccountId + "/regions/" + p.TargetRegion),
			}
		case nil:
			// skip nil provider
		default:
			return res, &cli.FriendlyError{Message: fmt.Sprintf("Internal error: cannot create environment with invalid type '%T'", provider)}
		}
	}
	return res, nil
}

func GetNamespace(envResource corerp.EnvironmentResource) string {
	switch v := envResource.Properties.Compute.(type) {
	case *corerp.KubernetesCompute:
		return *v.Namespace
	}
	return ""
}

func CheckIfRecipeExists(ctx context.Context, client clients.ApplicationsManagementClient, environmentName string, recipeName string, resourceType string) (corerp.EnvironmentResource, map[string]map[string]*corerp.EnvironmentRecipeProperties, error) {
	envResource, err := client.GetEnvDetails(ctx, environmentName)
	if err != nil {
		return corerp.EnvironmentResource{}, nil, err
	}

	recipeProperties := envResource.Properties.Recipes

	if recipeProperties[resourceType] == nil || recipeProperties[resourceType][recipeName] == nil {
		return corerp.EnvironmentResource{}, nil, &cli.FriendlyError{Message: fmt.Sprintf("resource type %q or recipe %q is not part of the environment %q ", resourceType, recipeName, environmentName)}
	}

	return envResource, recipeProperties, nil
}
