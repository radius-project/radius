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

	"github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// GetNamespace takes in an EnvironmentResource object and returns a string representing the namespace associated with the
// KubernetesCompute object, or an empty string if the Compute property is not a KubernetesCompute object.
func GetNamespace(envResource corerp.EnvironmentResource) string {
	switch v := envResource.Properties.Compute.(type) {
	case *corerp.KubernetesCompute:
		return *v.Namespace
	}
	return ""
}

// CheckIfRecipeExists checks if a given recipe exists in a given environment and returns the environment resource, recipe
// properties and an error if the recipe does not exist.
func CheckIfRecipeExists(ctx context.Context, client clients.ApplicationsManagementClient, environmentName string, recipeName string, resourceType string) (corerp.EnvironmentResource, map[string]map[string]corerp.RecipePropertiesClassification, error) {
	envResource, err := client.GetEnvironment(ctx, environmentName)
	if err != nil {
		return corerp.EnvironmentResource{}, nil, err
	}

	recipeProperties := envResource.Properties.Recipes

	if recipeProperties[resourceType] == nil || recipeProperties[resourceType][recipeName] == nil {
		return corerp.EnvironmentResource{}, nil, clierrors.Message("The resource type %q or recipe %q is not part of the environment %q.", resourceType, recipeName, environmentName)
	}

	return envResource, recipeProperties, nil
}

// InitializeClientFactory initializes a new v20231001preview.ClientFactory using the provided workspace context.
// It connects to the workspace and creates a new client factory with anonymous credentials.
// If the connection fails, it returns an error.
func InitializeClientFactory(ctx context.Context, workspace *workspaces.Workspace) (*v20231001preview.ClientFactory, error) {
	connection, err := workspace.Connect(ctx)
	if err != nil {
		return nil, err
	}

	clientOptions := sdk.NewClientOptions(connection)
	clientFactory, err := v20231001preview.NewClientFactory(&tokencredentials.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return clientFactory, nil
}

// InitializeRadiusCoreClientFactory initializes a new v20250801preview.ClientFactory using the provided workspace context.
// It connects to the workspace and creates a new client factory with anonymous credentials.
// If the connection fails, it returns an error.
func InitializeRadiusCoreClientFactory(ctx context.Context, workspace *workspaces.Workspace) (*v20250801preview.ClientFactory, error) {
	connection, err := workspace.Connect(ctx)
	if err != nil {
		return nil, err
	}

	clientOptions := sdk.NewClientOptions(connection)
	clientFactory, err := v20250801preview.NewClientFactory(&tokencredentials.AnonymousCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return clientFactory, nil
}

// PopulateRecipePackClients adds a RecipePacksClient to clientsByScope for
// every scope referenced by packIDs that is not already in the map.
// Callers seed the map with workspace-scope and default-scope clients before
// calling this function.
func PopulateRecipePackClients(
	ctx context.Context,
	workspace *workspaces.Workspace,
	clientsByScope map[string]*v20250801preview.RecipePacksClient,
	packIDs []string,
) error {
	for _, packIDStr := range packIDs {
		// This is the bicep reference for id, and cannot be invalid.
		packID, _ := resources.Parse(packIDStr)
		scope := packID.RootScope()
		if _, exists := clientsByScope[scope]; exists {
			continue
		}
		factory, err := InitializeRadiusCoreClientFactory(ctx, workspace)
		if err != nil {
			return err
		}
		clientsByScope[scope] = factory.NewRecipePacksClient()
	}

	return nil
}
