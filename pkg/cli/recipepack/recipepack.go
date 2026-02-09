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
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/containers:latest",
		},
		{
			Name:           "persistentvolumes",
			ResourceType:   "Radius.Compute/persistentVolumes",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/persistentvolumes:latest",
		},
		{
			Name:           "routes",
			ResourceType:   "Radius.Compute/routes",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/routes:latest",
		},
		{
			Name:           "secrets",
			ResourceType:   "Radius.Security/secrets",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/secrets:latest",
		},
	}
}

// NewSingletonRecipePackResource creates a RecipePackResource containing a single recipe for the given resource type.
func NewSingletonRecipePackResource(resourceType, recipeLocation string) corerpv20250801.RecipePackResource {
	bicepKind := corerpv20250801.RecipeKindBicep

	return corerpv20250801.RecipePackResource{
		Location: to.Ptr("global"),
		Properties: &corerpv20250801.RecipePackProperties{
			Recipes: map[string]*corerpv20250801.RecipeDefinition{
				resourceType: {
					RecipeKind:     &bicepKind,
					RecipeLocation: to.Ptr(recipeLocation),
				},
			},
		},
	}
}

// CreateSingletonRecipePacks creates singleton recipe packs (one per resource type) using a RecipePacksClient.
// It returns the list of full resource IDs of the created recipe packs.
func CreateSingletonRecipePacks(ctx context.Context, client *corerpv20250801.RecipePacksClient, resourceGroupName string) ([]string, error) {
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

// GetCoreResourceTypes returns the set of core resource types that require recipe packs.
func GetCoreResourceTypes() map[string]bool {
	defs := GetSingletonRecipePackDefinitions()
	types := make(map[string]bool, len(defs))
	for _, def := range defs {
		types[def.ResourceType] = true
	}
	return types
}

// IsSingletonRecipePackName checks if the given name matches a known singleton recipe pack name.
func IsSingletonRecipePackName(name string) bool {
	for _, def := range GetSingletonRecipePackDefinitions() {
		if def.Name == name {
			return true
		}
	}
	return false
}

// CollectResourceTypesFromRecipePacks queries the recipe packs client for each pack name
// and collects all resource types from their recipes. Returns a map of resource type to pack name.
func CollectResourceTypesFromRecipePacks(ctx context.Context, client *corerpv20250801.RecipePacksClient, packNames []string) (map[string]string, error) {
	coveredTypes := make(map[string]string)
	for _, name := range packNames {
		resp, err := client.Get(ctx, name, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get recipe pack %q: %w", name, err)
		}
		if resp.Properties != nil && resp.Properties.Recipes != nil {
			for resourceType := range resp.Properties.Recipes {
				coveredTypes[resourceType] = name
			}
		}
	}
	return coveredTypes, nil
}

// DetectResourceTypeConflicts checks if any resource type appears in multiple recipe packs.
// Returns a map of resource type to list of pack names that contain it.
func DetectResourceTypeConflicts(ctx context.Context, client *corerpv20250801.RecipePacksClient, packNames []string) (map[string][]string, error) {
	typeToPackNames := make(map[string][]string)
	for _, name := range packNames {
		resp, err := client.Get(ctx, name, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get recipe pack %q: %w", name, err)
		}
		if resp.Properties != nil && resp.Properties.Recipes != nil {
			for resourceType := range resp.Properties.Recipes {
				typeToPackNames[resourceType] = append(typeToPackNames[resourceType], name)
			}
		}
	}
	conflicts := make(map[string][]string)
	for resourceType, packs := range typeToPackNames {
		if len(packs) > 1 {
			conflicts[resourceType] = packs
		}
	}
	return conflicts, nil
}

// GetMissingSingletonDefinitions returns singleton definitions for core resource types
// that are not already covered by the existing recipe packs.
func GetMissingSingletonDefinitions(coveredTypes map[string]string) []SingletonRecipePackDefinition {
	var missing []SingletonRecipePackDefinition
	for _, def := range GetSingletonRecipePackDefinitions() {
		if _, covered := coveredTypes[def.ResourceType]; !covered {
			missing = append(missing, def)
		}
	}
	return missing
}

// CreateMissingSingletonRecipePacks creates singleton recipe packs for core resource types
// that are not already covered by existing recipe packs. Returns the IDs of created packs.
func CreateMissingSingletonRecipePacks(ctx context.Context, client *corerpv20250801.RecipePacksClient, resourceGroupName string, coveredTypes map[string]string) ([]string, error) {
	missing := GetMissingSingletonDefinitions(coveredTypes)
	if len(missing) == 0 {
		return nil, nil
	}

	createdIDs := make([]string, 0, len(missing))
	for _, def := range missing {
		resource := NewSingletonRecipePackResource(def.ResourceType, def.RecipeLocation)
		_, err := client.CreateOrUpdate(ctx, def.Name, resource, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create recipe pack %q for resource type %q: %w", def.Name, def.ResourceType, err)
		}
		recipePackID := fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/Radius.Core/recipePacks/%s", resourceGroupName, def.Name)
		createdIDs = append(createdIDs, recipePackID)
	}

	return createdIDs, nil
}
