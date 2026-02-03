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

package recipepack

import (
	"context"
	"fmt"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
)

const (
	// DefaultRecipePackName is the name of the default Kubernetes recipe pack.
	DefaultRecipePackName = "local-dev"
)

// CreateDefaultRecipePackWithClient creates the default Kubernetes recipe pack using a RecipePacksClient.
// It returns the full resource ID of the created recipe pack.
func CreateDefaultRecipePackWithClient(ctx context.Context, client *corerpv20250801.RecipePacksClient, resourceGroupName string) (string, error) {
	resource := NewDefaultRecipePackResource()
	_, err := client.CreateOrUpdate(ctx, DefaultRecipePackName, resource, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create recipe pack: %w", err)
	}

	// Return the full resource ID of the created recipe pack
	recipePackID := fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Radius.Core/recipePacks/%s", resourceGroupName, DefaultRecipePackName)
	return recipePackID, nil
}

// NewDefaultRecipePackResource builds the default RecipePackResource containing
// Bicep recipes for the built-in Radius resource types.
func NewDefaultRecipePackResource() corerpv20250801.RecipePackResource {
	bicepKind := corerpv20250801.RecipeKindBicep
	plainHTTP := true

	return corerpv20250801.RecipePackResource{
		Location: to.Ptr("global"),
		Properties: &corerpv20250801.RecipePackProperties{
			Recipes: map[string]*corerpv20250801.RecipeDefinition{
				"Radius.Compute/containers": {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr("localhost:5000/radius-recipes/compute/containers/kubernetes/bicep/kubernetes-containers:latest"),
					PlainHTTP:      &plainHTTP,
				},
				"Radius.Compute/persistentVolumes": {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr("localhost:5000/radius-recipes/compute/persistentvolumes/kubernetes/bicep/kubernetes-volumes:latest"),
					PlainHTTP:      &plainHTTP,
				},
				"Radius.Data/mySqlDatabases": {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr("localhost:5000/radius-recipes/data/mysqldatabases/kubernetes/bicep/kubernetes-mysql:latest"),
					PlainHTTP:      &plainHTTP,
				},
				"Radius.Data/postgreSqlDatabases": {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr("localhost:5000/radius-recipes/data/postgresqldatabases/kubernetes/bicep/kubernetes-postgresql:latest"),
					PlainHTTP:      &plainHTTP,
				},
				"Radius.Security/secrets": {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr("localhost:5000/radius-recipes/security/secrets/kubernetes/bicep/kubernetes-secrets:latest"),
					PlainHTTP:      &plainHTTP,
				},
			},
		},
	}
}

// CreateRecipePack creates a recipe pack in the specified resource group.
// It returns the full resource ID of the created recipe pack.
func CreateRecipePack(ctx context.Context, connection sdk.Connection, resourceGroupName string, recipePackName string, resource corerpv20250801.RecipePackResource) (string, error) {
	clientOptions := sdk.NewClientOptions(connection)

	rpClient, err := corerpv20250801.NewRecipePacksClient(
		fmt.Sprintf("planes/radius/local/resourceGroups/%s", resourceGroupName),
		&aztoken.AnonymousCredential{},
		clientOptions,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create recipe pack client: %w", err)
	}

	_, err = rpClient.CreateOrUpdate(ctx, recipePackName, resource, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create recipe pack: %w", err)
	}

	// Return the full resource ID of the created recipe pack
	recipePackID := fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Radius.Core/recipePacks/%s", resourceGroupName, recipePackName)
	return recipePackID, nil
}

// CreateDefaultRecipePack creates the default Kubernetes recipe pack in the specified resource group.
// It returns the full resource ID of the created recipe pack.
func CreateDefaultRecipePack(ctx context.Context, connection sdk.Connection, resourceGroupName string) (string, error) {
	return CreateRecipePack(ctx, connection, resourceGroupName, DefaultRecipePackName, NewDefaultRecipePackResource())
}
