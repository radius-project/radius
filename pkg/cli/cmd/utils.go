// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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

func CheckIfRecipeExists(ctx context.Context, client clients.ApplicationsManagementClient, environmentName string, recipeName string) (corerp.EnvironmentResource, map[string]*corerp.EnvironmentRecipeProperties, error) {
	envResource, err := client.GetEnvDetails(ctx, environmentName)
	if err != nil {
		return corerp.EnvironmentResource{}, nil, err
	}

	recipeProperties := envResource.Properties.Recipes

	if recipeProperties[recipeName] == nil {
		return corerp.EnvironmentResource{}, nil, &cli.FriendlyError{Message: fmt.Sprintf("recipe %q is not part of the environment %q ", recipeName, environmentName)}
	}

	return envResource, recipeProperties, nil
}
