// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
)

// CreateEnvAzureProvider forms the azure provider scope from the subscriptionID and resourceGroup
func CreateEnvAzureProvider(subscriptionID, resourceGroup string) corerp.Providers {
	providers := corerp.Providers{
		Azure: &corerp.ProvidersAzure{
			Scope: to.Ptr("/subscriptions/" + subscriptionID + "/resourceGroups/" + resourceGroup),
		},
	}
	return providers
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
