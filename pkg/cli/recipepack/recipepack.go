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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// DefaultRecipePackResourceName is the name of the default recipe pack
	// resource that contains recipes for all core resource types.
	DefaultRecipePackResourceName = "default"

	// DefaultResourceGroupName is the name of the default resource group where
	// singleton recipe packs are created and looked up.
	DefaultResourceGroupName = "default"

	// DefaultResourceGroupScope is the full scope path for the default resource group.
	// Singleton recipe packs that Radius provides by default always live in this scope.
	DefaultResourceGroupScope = "/planes/radius/local/resourceGroups/" + DefaultResourceGroupName
)

// ResourceGroupCreator is a function that creates or updates a Radius resource group.
// This is typically satisfied by ApplicationsManagementClient.CreateOrUpdateResourceGroup.
type ResourceGroupCreator func(ctx context.Context, planeName string, resourceGroupName string, resource *ucpv20231001.ResourceGroupResource) error

// NewDefaultRecipePackResource creates a RecipePackResource containing recipes
// for all core resource types. This is the default recipe pack that gets injected into
// environments that have no recipe packs configured.
func NewDefaultRecipePackResource() corerpv20250801.RecipePackResource {
	bicepKind := corerpv20250801.RecipeKindBicep
	recipes := make(map[string]*corerpv20250801.RecipeDefinition)
	for _, def := range GetDefaultRecipePackDefinition() {
		recipes[def.ResourceType] = &corerpv20250801.RecipeDefinition{
			RecipeKind:     &bicepKind,
			RecipeLocation: to.Ptr(def.RecipeLocation),
		}
	}
	return corerpv20250801.RecipePackResource{
		Location: to.Ptr("global"),
		Properties: &corerpv20250801.RecipePackProperties{
			Recipes: recipes,
		},
	}
}

// DefaultRecipePackID returns the full resource ID of the default recipe pack
// in the default resource group scope.
func DefaultRecipePackID() string {
	return fmt.Sprintf("%s/providers/Radius.Core/recipePacks/%s", DefaultResourceGroupScope, DefaultRecipePackResourceName)
}

// EnsureDefaultResourceGroup creates the default resource group if it does not already exist.
// This must be called before creating singleton recipe packs, because recipe packs are
// stored in the default resource group and the PUT will fail with 404 if the group is missing.
// The group might be missing in a sequence such as below:
// 1. rad install
// 2. rad workspace create kubernetes
// 3. rad group create prod
// 4. rad group switch prod
// 5. .rad deploy <template contains the environment>
func EnsureDefaultResourceGroup(ctx context.Context, createOrUpdate ResourceGroupCreator) error {
	return createOrUpdate(ctx, "local", DefaultResourceGroupName, &ucpv20231001.ResourceGroupResource{
		Location: to.Ptr(v1.LocationGlobal),
	})
}

// SingletonRecipePackDefinition defines a singleton recipe pack for a single resource type.
type SingletonRecipePackDefinition struct {
	// Name is the name of the recipe pack (derived from resource type).
	Name string
	// ResourceType is the full resource type (e.g., "Radius.Compute/containers").
	ResourceType string
	// RecipeLocation is the OCI registry location for the recipe.
	RecipeLocation string
}

// GetDefaultRecipePackDefinition returns the list of singleton recipe pack definitions.
// Each definition represents a single recipe pack containing one recipe for one resource type.
// This list is currently hardcoded, but will be made dynamic in the future.
func GetDefaultRecipePackDefinition() []SingletonRecipePackDefinition {
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
// The client must be scoped to the default resource group (DefaultResourceGroupScope).
// It returns the list of full resource IDs of the created recipe packs, always in the default scope.
func CreateSingletonRecipePacks(ctx context.Context, client *corerpv20250801.RecipePacksClient) ([]string, error) {
	definitions := GetDefaultRecipePackDefinition()
	recipePackIDs := make([]string, 0, len(definitions))

	for _, def := range definitions {
		resource := NewSingletonRecipePackResource(def.ResourceType, def.RecipeLocation)
		_, err := client.CreateOrUpdate(ctx, def.Name, resource, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create recipe pack %q for resource type %q: %w", def.Name, def.ResourceType, err)
		}

		// Return the full resource ID of the created recipe pack in the default scope.
		recipePackID := fmt.Sprintf("%s/providers/Radius.Core/recipePacks/%s", DefaultResourceGroupScope, def.Name)
		recipePackIDs = append(recipePackIDs, recipePackID)
	}

	return recipePackIDs, nil
}

// GetCoreResourceTypes returns the set of core resource types that require recipe packs.
func GetCoreResourceTypes() map[string]bool {
	defs := GetDefaultRecipePackDefinition()
	types := make(map[string]bool, len(defs))
	for _, def := range defs {
		types[def.ResourceType] = true
	}
	return types
}

// IsSingletonRecipePackName checks if the given name matches a known singleton recipe pack name.
func IsSingletonRecipePackName(name string) bool {
	for _, def := range GetDefaultRecipePackDefinition() {
		if def.Name == name {
			return true
		}
	}
	return false
}

// CollectResourceTypesFromRecipePacks queries the recipe packs client for each pack name
// and collects all resource types from their recipes. Returns a map of resource type to pack name.
// func CollectResourceTypesFromRecipePacks(ctx context.Context, client *corerpv20250801.RecipePacksClient, packNames []string) (map[string]string, error) {
// 	coveredTypes := make(map[string]string)
// 	for _, name := range packNames {
// 		resp, err := client.Get(ctx, name, nil)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to get recipe pack %q: %w", name, err)
// 		}
// 		if resp.Properties != nil && resp.Properties.Recipes != nil {
// 			for resourceType := range resp.Properties.Recipes {
// 				coveredTypes[resourceType] = name
// 			}
// 		}
// 	}
// 	return coveredTypes, nil
// }

// DetectResourceTypeConflicts checks if any resource type appears in multiple recipe packs.
// Returns a map of resource type to list of pack names that contain it.
// func DetectResourceTypeConflicts(ctx context.Context, client *corerpv20250801.RecipePacksClient, packNames []string) (map[string][]string, error) {
// 	typeToPackNames := make(map[string][]string)
// 	for _, name := range packNames {
// 		resp, err := client.Get(ctx, name, nil)
// 		if err != nil {
// 			return nil, fmt.Errorf("failed to get recipe pack %q: %w", name, err)
// 		}
// 		if resp.Properties != nil && resp.Properties.Recipes != nil {
// 			for resourceType := range resp.Properties.Recipes {
// 				typeToPackNames[resourceType] = append(typeToPackNames[resourceType], name)
// 			}
// 		}
// 	}
// 	conflicts := make(map[string][]string)
// 	for resourceType, packs := range typeToPackNames {
// 		if len(packs) > 1 {
// 			conflicts[resourceType] = packs
// 		}
// 	}
// 	return conflicts, nil
// }

// GetMissingSingletonDefinitions returns singleton definitions for core resource types
// that are not already covered by the existing recipe packs.
func GetMissingSingletonDefinitions(coveredTypes map[string]string) []SingletonRecipePackDefinition {
	var missing []SingletonRecipePackDefinition
	for _, def := range GetDefaultRecipePackDefinition() {
		if _, covered := coveredTypes[def.ResourceType]; !covered {
			missing = append(missing, def)
		}
	}
	return missing
}

// CreateMissingSingletonRecipePacks creates singleton recipe packs for core resource types
// that are not already covered by existing recipe packs. The client must be scoped to the
// default resource group. Returns the IDs of created packs, always in the default scope.
func CreateMissingSingletonRecipePacks(ctx context.Context, client *corerpv20250801.RecipePacksClient, coveredTypes map[string]string) ([]string, error) {
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
		recipePackID := fmt.Sprintf("%s/providers/Radius.Core/recipePacks/%s", DefaultResourceGroupScope, def.Name)
		createdIDs = append(createdIDs, recipePackID)
	}

	return createdIDs, nil
}

// ---------------------------------------------------------------------------
// Shared utilities used by env create, env update, and rad deploy commands.
// ---------------------------------------------------------------------------

// InspectRecipePacks fetches each recipe pack by its full resource ID,
// collects the resource types each pack provides, and detects conflicts where
// the same resource type appears in more than one pack.
//
// clientsByScope maps root scope strings to a RecipePacksClient for that scope.
// Pack IDs whose scope has no matching client, or that cannot be parsed, are
// silently skipped.
func InspectRecipePacks(ctx context.Context, clientsByScope map[string]*corerpv20250801.RecipePacksClient, packIDs []string) (coveredTypes map[string]string, conflicts map[string][]string, err error) {
	typeToPacks := make(map[string][]string)
	coveredTypes = make(map[string]string)

	for _, packIDStr := range packIDs {
		packID, parseErr := resources.Parse(packIDStr)
		if parseErr != nil {
			continue
		}

		client, ok := clientsByScope[packID.RootScope()]
		if !ok {
			continue
		}

		resp, err := client.Get(ctx, packID.Name(), nil)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to inspect recipe pack %q: %w", packIDStr, err)
		}

		if resp.Properties != nil && resp.Properties.Recipes != nil {
			for resourceType := range resp.Properties.Recipes {
				typeToPacks[resourceType] = append(typeToPacks[resourceType], packID.Name())
				if _, exists := coveredTypes[resourceType]; !exists {
					coveredTypes[resourceType] = packID.Name()
				}
			}
		}
	}

	conflicts = make(map[string][]string)
	for resourceType, packs := range typeToPacks {
		if len(packs) > 1 {
			conflicts[resourceType] = packs
		}
	}

	return coveredTypes, conflicts, nil
}

// FormatConflictError creates a user-friendly error when resource types are
// provided by multiple recipe packs.
func FormatConflictError(conflicts map[string][]string) error {
	var b strings.Builder
	b.WriteString("Recipe pack conflict detected. The following resource types are provided by multiple recipe packs:\n")
	for resourceType, packs := range conflicts {
		fmt.Fprintf(&b, "  - %s: provided by packs %v\n", resourceType, packs)
	}
	b.WriteString("\nPlease resolve these conflicts by removing or replacing conflicting recipe packs.")
	return fmt.Errorf("%s", b.String())
}

// EnsureMissingSingletons creates (or updates, idempotently) singleton recipe
// pack resources for core resource types not already covered by coveredTypes,
// and returns their full resource IDs in the default resource group scope.
// The client must be scoped to DefaultResourceGroupScope.
// Singletons always live in the default scope.
func EnsureMissingSingletons(ctx context.Context, client *corerpv20250801.RecipePacksClient, coveredTypes map[string]string) ([]string, error) {
	missing := GetMissingSingletonDefinitions(coveredTypes)
	if len(missing) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(missing))
	for _, def := range missing {
		resource := NewSingletonRecipePackResource(def.ResourceType, def.RecipeLocation)
		_, err := client.CreateOrUpdate(ctx, def.Name, resource, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create recipe pack %q for resource type %q: %w", def.Name, def.ResourceType, err)
		}
		ids = append(ids, fmt.Sprintf("%s/providers/Radius.Core/recipePacks/%s", DefaultResourceGroupScope, def.Name))
	}
	return ids, nil
}

// RecipePackIDExists checks whether id is present in a []*string slice.
func RecipePackIDExists(packs []*string, id string) bool {
	for _, p := range packs {
		if p != nil && *p == id {
			return true
		}
	}
	return false
}

// ExtractRecipePackIDs extracts recipe pack IDs from an ARM template's
// properties["recipePacks"] value, which is typed as []any after JSON
// deserialization. Only literal string elements are returned.
func ExtractRecipePackIDs(properties map[string]any) []string {
	var ids []string
	recipePacks, ok := properties["recipePacks"]
	if !ok {
		return ids
	}

	packsArray, ok := recipePacks.([]any)
	if !ok {
		return ids
	}

	for _, p := range packsArray {
		s, ok := p.(string)
		if !ok {
			continue
		}

		// Skip ARM template expressions like "[reference('mypack').id]".
		// These are runtime references to other resources in the template
		// and cannot be parsed as resource IDs.
		if strings.HasPrefix(s, "[") {
			continue
		}

		ids = append(ids, s)
	}
	return ids
}
