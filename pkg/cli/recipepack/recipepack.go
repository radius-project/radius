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
	"strings"

	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
)

const (
	// DefaultRecipePackName is the name of the default Kubernetes recipe pack.
	DefaultRecipePackName = "local-dev"
)

// SingletonRecipePackDefinition defines a singleton recipe pack for a single resource type.
type SingletonRecipePackDefinition struct {
	// Name is the name of the recipe pack (derived from resource type).
	Name string
	// ResourceType is the full resource type (e.g., "Radius.Compute/containers").
	ResourceType string
	// RecipeLocation is the OCI registry location for the recipe.
	RecipeLocation string
}

// GetSingletonRecipePackDefinitions returns the list of singleton recipe pack definitions.
// Each definition represents a single recipe pack containing one recipe for one resource type.
func GetSingletonRecipePackDefinitions() []SingletonRecipePackDefinition {
	return []SingletonRecipePackDefinition{
		{
			Name:           "containers",
			ResourceType:   "Radius.Compute/containers",
			RecipeLocation: "localhost:5000/radius-recipes/compute/containers/kubernetes/bicep/kubernetes-containers:latest",
		},
		{
			Name:           "persistentvolumes",
			ResourceType:   "Radius.Compute/persistentVolumes",
			RecipeLocation: "localhost:5000/radius-recipes/compute/persistentvolumes/kubernetes/bicep/kubernetes-volumes:latest",
		},
		{
			Name:           "mysqldatabases",
			ResourceType:   "Radius.Data/mySqlDatabases",
			RecipeLocation: "localhost:5000/radius-recipes/data/mysqldatabases/kubernetes/bicep/kubernetes-mysql:latest",
		},
		{
			Name:           "postgresqldatabases",
			ResourceType:   "Radius.Data/postgreSqlDatabases",
			RecipeLocation: "localhost:5000/radius-recipes/data/postgresqldatabases/kubernetes/bicep/kubernetes-postgresql:latest",
		},
		{
			Name:           "secrets",
			ResourceType:   "Radius.Security/secrets",
			RecipeLocation: "localhost:5000/radius-recipes/security/secrets/kubernetes/bicep/kubernetes-secrets:latest",
		},
	}
}

// NewSingletonRecipePackResource creates a RecipePackResource containing a single recipe for the given resource type.
func NewSingletonRecipePackResource(resourceType, recipeLocation string) corerpv20250801.RecipePackResource {
	bicepKind := corerpv20250801.RecipeKindBicep
	plainHTTP := true

	return corerpv20250801.RecipePackResource{
		Location: to.Ptr("global"),
		Properties: &corerpv20250801.RecipePackProperties{
			Recipes: map[string]*corerpv20250801.RecipeDefinition{
				resourceType: {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr(recipeLocation),
					PlainHTTP:      &plainHTTP,
				},
			},
		},
	}
}

// CreateSingletonRecipePacksWithClient creates singleton recipe packs (one per resource type) using a RecipePacksClient.
// It returns the list of full resource IDs of the created recipe packs.
func CreateSingletonRecipePacksWithClient(ctx context.Context, client *corerpv20250801.RecipePacksClient, resourceGroupName string) ([]string, error) {
	definitions := GetSingletonRecipePackDefinitions()
	recipePackIDs := make([]string, 0, len(definitions))

	for _, def := range definitions {
		resource := NewSingletonRecipePackResource(def.ResourceType, def.RecipeLocation)
		_, err := client.CreateOrUpdate(ctx, def.Name, resource, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create recipe pack %q for resource type %q: %w", def.Name, def.ResourceType, err)
		}

		// Return the full resource ID of the created recipe pack
		recipePackID := fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Radius.Core/recipePacks/%s", resourceGroupName, def.Name)
		recipePackIDs = append(recipePackIDs, recipePackID)
	}

	return recipePackIDs, nil
}

// GetRecipePackNameForResourceType returns the recipe pack name for a given resource type.
// Returns empty string if the resource type is not found.
func GetRecipePackNameForResourceType(resourceType string) string {
	for _, def := range GetSingletonRecipePackDefinitions() {
		if strings.EqualFold(def.ResourceType, resourceType) {
			return def.Name
		}
	}
	return ""
}

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

