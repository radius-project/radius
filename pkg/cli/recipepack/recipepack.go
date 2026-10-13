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
	"errors"
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/defaults"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_radius "github.com/radius-project/radius/pkg/ucp/resources/radius"
	"github.com/radius-project/radius/pkg/version"
)

// ResourceType is the resource type of a recipe pack.
const ResourceType = "Radius.Core/recipePacks"

// ResolveID resolves a recipe pack reference to a full resource ID. The reference
// may be a full recipe pack resource ID, in which case it is used as-is and isFullID
// is true, or a bare name, which is scoped to workspaceScope. A value that parses as
// a resource ID but is not a Radius.Core/recipePacks resource is rejected, so that a
// mistyped ID of another type is not silently looked up as a recipe pack name.
func ResolveID(recipePack string, workspaceScope string) (id resources.ID, isFullID bool, err error) {
	if recipePackID, parseErr := resources.Parse(recipePack); parseErr == nil {
		if !recipePackID.IsResource() || !strings.EqualFold(recipePackID.Type(), ResourceType) {
			return resources.ID{}, false, clierrors.Message("%q is not a recipe pack resource ID. Provide a recipe pack name, or a resource ID of the form /planes/radius/local/resourceGroups/<group>/providers/%s/<name>.", recipePack, ResourceType)
		}

		return recipePackID, true, nil
	}

	scopeID, err := resources.ParseScope(workspaceScope)
	if err != nil {
		return resources.ID{}, false, err
	}

	return scopeID.Append(resources.TypeSegment{
		Type: ResourceType,
		Name: recipePack,
	}), false, nil
}

// NotFoundError builds the error returned when a recipe pack passed to --recipe-packs
// cannot be found. When the user supplied a bare name, the message names the resource
// group that was searched and shows how to reference a pack in another resource group.
func NotFoundError(recipePack string, recipePackID resources.ID, isFullID bool) error {
	resourceGroup := recipePackID.FindScope(resources_radius.ScopeResourceGroups)
	if isFullID || resourceGroup == "" {
		return clierrors.Message("Recipe pack %q does not exist. Please provide a valid recipe pack to set on the environment.", recipePack)
	}

	return clierrors.Message("Recipe pack %q does not exist in resource group %q. To reference a recipe pack in another resource group, pass its full resource ID, for example: %s",
		recipePack, resourceGroup, recipePackID.String())
}

// NormalizeRecipePacks splits comma-separated values, trims whitespace, and
// removes empty entries and duplicates while preserving the first-seen order.
// Deduplication avoids redundant referencedBy sync work and prevents server-side
// recipe pack conflict validation from failing on repeated entries.
func NormalizeRecipePacks(recipePacks []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range recipePacks {
		for p := range strings.SplitSeq(value, ",") {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			if _, ok := seen[trimmed]; ok {
				continue
			}
			seen[trimmed] = struct{}{}
			result = append(result, trimmed)
		}
	}
	return result
}

// RefExists reports whether id is present in the referencedBy list.
func RefExists(environmentRefs []*string, id string) bool {
	for _, ref := range environmentRefs {
		if ref != nil && *ref == id {
			return true
		}
	}
	return false
}

const (
	// DefaultRecipePackResourceName is the name of the Radius provided
	// recipe pack resource that contains kubernetes recipes for all core resource types.
	DefaultRecipePackResourceName = "default"

	// DefaultResourceGroupName is the name of the default resource group where
	// the default recipe pack is created and looked up.
	DefaultResourceGroupName = "default"

	// DefaultResourceGroupScope is the full scope path for the default resource group.
	// default recipe pack that Radius provides always live in this scope.
	DefaultResourceGroupScope = "/planes/radius/local/resourceGroups/" + DefaultResourceGroupName
)

// ResourceGroupCreator is a function that creates or updates a Radius resource group.
// This is typically satisfied by ApplicationsManagementClient.CreateOrUpdateResourceGroup.
type ResourceGroupCreator func(ctx context.Context, planeName string, resourceGroupName string, resource *ucpv20231001.ResourceGroupResource) error

// NewDefaultRecipePackResource creates the default recipe pack that gets
// injected into environments that have no recipe packs configured. It is the
// Kubernetes recipe pack embedded from the resource-types-contrib revision
// pinned in deploy/manifest/defaults.yaml, as authored upstream; see
// defaults.DefaultKubernetesRecipePack.
//
// The only change rad makes is the tag of each recipe source, which is replaced
// with the tag for this build's channel; see resolveRecipeTag.
func NewDefaultRecipePackResource() (corerpv20250801.RecipePackResource, error) {
	pack, err := defaults.DefaultKubernetesRecipePack()
	if err != nil {
		return corerpv20250801.RecipePackResource{}, fmt.Errorf("the default Kubernetes recipe pack is not available in this build of rad: %w", err)
	}
	return newDefaultRecipePackResource(pack, version.IsEdgeChannel())
}

// newDefaultRecipePackResource builds the default recipe pack from pack for
// the given channel, retagging the recipe sources in place. The channel is a
// parameter so both branches are reachable from tests.
func newDefaultRecipePackResource(pack *corerpv20250801.RecipePackProperties, isEdge bool) (corerpv20250801.RecipePackResource, error) {
	if pack == nil || len(pack.Recipes) == 0 {
		return corerpv20250801.RecipePackResource{}, errors.New("the default Kubernetes recipe pack has no recipes")
	}

	for resourceType, recipe := range pack.Recipes {
		if recipe == nil || recipe.Source == nil {
			return corerpv20250801.RecipePackResource{}, fmt.Errorf("default recipe for %s has no source", resourceType)
		}
		repository, err := defaults.RecipeSourceRepository(*recipe.Source)
		if err != nil {
			return corerpv20250801.RecipePackResource{}, fmt.Errorf("default recipe for %s: %w", resourceType, err)
		}
		recipe.Source = to.Ptr(repository + ":" + resolveRecipeTag(resourceType, isEdge))
	}
	return corerpv20250801.RecipePackResource{
		Location:   to.Ptr("global"),
		Properties: pack,
	}, nil
}

// DefaultRecipePackID returns the full resource ID of the default recipe pack
// in the default resource group scope.
func DefaultRecipePackID() string {
	return fmt.Sprintf("%s/providers/Radius.Core/recipePacks/%s", DefaultResourceGroupScope, DefaultRecipePackResourceName)
}

// EnsureDefaultResourceGroup creates the default resource group if it does not already exist.
// This must be called before creating the default recipe pack, because recipe packs are
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

// GetOrCreateDefaultRecipePack attempts to GET the default recipe pack from
// the default scope. If it doesn't exist (404), it creates it from
// NewDefaultRecipePackResource. Returns the full resource ID.
func GetOrCreateDefaultRecipePack(ctx context.Context, client *corerpv20250801.RecipePacksClient) (string, error) {
	_, err := client.Get(ctx, DefaultResourceGroupScope, DefaultRecipePackResourceName, nil)
	if err != nil {
		if !clients.Is404Error(err) {
			return "", fmt.Errorf("failed to get default recipe pack from default scope: %w", err)
		}
		// Not found — create the default recipe pack.
		resource, err := NewDefaultRecipePackResource()
		if err != nil {
			return "", fmt.Errorf("failed to build default recipe pack: %w", err)
		}
		_, err = client.CreateOrUpdate(ctx, DefaultResourceGroupScope, DefaultRecipePackResourceName, resource, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create default recipe pack: %w", err)
		}
	}
	return DefaultRecipePackID(), nil
}

// resolveRecipeTag picks the OCI tag for a single default recipe from the build
// channel and the per-namespace pin recorded in deploy/manifest/defaults.yaml
// under `resourceTypes`:
//
//   - Edge / dev builds (isEdge) always use the mutable tag "edge" so a
//     locally-built CLI picks up whatever the recipes publishing pipeline last
//     pushed for the tip of the recipes repository.
//   - Release builds resolve the recipe's resource-type namespace (e.g.
//     "Radius.Compute" for "Radius.Compute/containers") to the immutable commit
//     SHA that defaults.yaml pins that namespace to, via
//     defaults.ResourceTypePin. The recipes publishing pipeline tags each
//     namespace's OCI artifacts with the same commit SHA, so a released rad CLI
//     installs exactly the recipes published from the pinned revision.
//   - As a defensive fallback, a namespace missing from the defaults.yaml
//     `resourceTypes` list falls back to "edge" so a mis-configured build still
//     installs something rather than producing an invalid OCI reference.
//
// isEdge is passed in rather than read from the build-stamped channel so both
// branches are reachable from tests.
func resolveRecipeTag(resourceType string, isEdge bool) string {
	if isEdge {
		return "edge"
	}
	namespace, _, ok := defaults.SplitResourceType(resourceType)
	if !ok {
		return "edge"
	}
	pin, ok := defaults.ResourceTypePin(namespace)
	if !ok || pin.Ref == "" {
		return "edge"
	}
	return pin.Ref
}
