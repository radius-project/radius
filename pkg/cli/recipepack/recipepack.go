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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/version"
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

// GetDefaultRecipePackDefinition returns the list of default recipe pack definitions.
// Each definition represents a recipe for one core resource type.
// The OCI tag is set to the current Radius version channel (e.g., "0.40" or "edge").
func GetDefaultRecipePackDefinition() []SingletonRecipePackDefinition {
	tag := version.Channel()
	if version.IsEdgeChannel() {
		tag = "latest"
	}
	return []SingletonRecipePackDefinition{
		{
			Name:           "containers",
			ResourceType:   "Radius.Compute/containers",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/containers:" + tag,
		},
		{
			Name:           "persistentvolumes",
			ResourceType:   "Radius.Compute/persistentVolumes",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/persistentvolumes:" + tag,
		},
		{
			Name:           "routes",
			ResourceType:   "Radius.Compute/routes",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/routes:" + tag,
		},
		{
			Name:           "secrets",
			ResourceType:   "Radius.Security/secrets",
			RecipeLocation: "ghcr.io/radius-project/kube-recipes/secrets:" + tag,
		},
	}
}
