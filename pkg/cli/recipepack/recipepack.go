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
	"sync"

	"github.com/radius-project/radius/deploy/manifest"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/version"
)

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

	// recipeRegistryPrefix is the OCI repository prefix under which Radius
	// publishes the default Bicep recipes for core resource types. The full
	// location for a type is <prefix>/<lowercased-typename>:<tag>.
	recipeRegistryPrefix = "ghcr.io/radius-project/kube-recipes"

	// namespacePrefix is the literal prefix every default-registered resource
	// type namespace must start with. Entries lacking this prefix are rejected
	// as malformed.
	namespacePrefix = "Radius."
)

// ResourceGroupCreator is a function that creates or updates a Radius resource group.
// This is typically satisfied by ApplicationsManagementClient.CreateOrUpdateResourceGroup.
type ResourceGroupCreator func(ctx context.Context, planeName string, resourceGroupName string, resource *ucpv20231001.ResourceGroupResource) error

// CoreTypesRecipeInfo defines a recipe entry for a single resource type in the default recipe pack.
type CoreTypesRecipeInfo struct {
	// ResourceType is the full resource type (e.g., "Radius.Compute/containers").
	ResourceType string
	// RecipeLocation is the OCI registry location for the recipe.
	RecipeLocation string
}

// coreTypesCache memoizes the parsed-and-validated default recipe info so
// the embedded YAML is parsed at most once per process. coreTypesErr captures
// any validation failure discovered on that first call.
var (
	coreTypesOnce  sync.Once
	coreTypesCache []CoreTypesRecipeInfo
	coreTypesErr   error
)

// NewDefaultRecipePackResource creates a RecipePackResource containing recipes
// for all core resource types listed in deploy/manifest/defaults.yaml.
//
// The recipe entries are derived from defaults.yaml at first call: for each
// entry "Radius.<Namespace>/<typeName>" the location is
// "ghcr.io/radius-project/kube-recipes/<lowercased-typename>:<tag>" and the
// kind is always Bicep.
//
// This function panics if defaults.yaml is missing, empty, or contains a
// malformed entry. Such a failure indicates a build-time misconfiguration of
// the embedded YAML and is caught by unit tests / CI; it is not a recoverable
// runtime condition.
func NewDefaultRecipePackResource() corerpv20250801.RecipePackResource {
	recipeDefinitions, err := loadCoreTypesRecipeInfo()
	if err != nil {
		panic(fmt.Sprintf("recipepack: %v", err))
	}

	bicepKind := corerpv20250801.RecipeKindBicep
	recipes := make(map[string]*corerpv20250801.RecipeDefinition, len(recipeDefinitions))
	for _, recipeDef := range recipeDefinitions {
		recipes[recipeDef.ResourceType] = &corerpv20250801.RecipeDefinition{
			RecipeKind:     &bicepKind,
			RecipeLocation: to.Ptr(recipeDef.RecipeLocation),
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
// the default scope. If it doesn't exist (404), it creates it with all core
// resource type recipes. Returns the full resource ID.
func GetOrCreateDefaultRecipePack(ctx context.Context, client *corerpv20250801.RecipePacksClient) (string, error) {
	_, err := client.Get(ctx, DefaultRecipePackResourceName, nil)
	if err != nil {
		if !clients.Is404Error(err) {
			return "", fmt.Errorf("failed to get default recipe pack from default scope: %w", err)
		}
		// Not found — create the default recipe pack with all core types.
		resource := NewDefaultRecipePackResource()
		_, err = client.CreateOrUpdate(ctx, DefaultRecipePackResourceName, resource, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create default recipe pack: %w", err)
		}
	}
	return DefaultRecipePackID(), nil
}

// GetCoreTypesRecipeInfo returns recipe information for all core types, derived
// from deploy/manifest/defaults.yaml.
//
// Each definition represents a recipe for one core resource type. The OCI tag
// is set to the current Radius version channel (e.g., "0.40") or "latest" when
// running on the edge channel.
//
// This function panics if defaults.yaml is missing, empty, or contains a
// malformed entry. Such a failure indicates a build-time misconfiguration of
// the embedded YAML and is caught by unit tests / CI.
func GetCoreTypesRecipeInfo() []CoreTypesRecipeInfo {
	infos, err := loadCoreTypesRecipeInfo()
	if err != nil {
		panic(fmt.Sprintf("recipepack: %v", err))
	}
	// Return a defensive copy so callers cannot mutate the cached slice.
	out := make([]CoreTypesRecipeInfo, len(infos))
	copy(out, infos)
	return out
}

// loadCoreTypesRecipeInfo parses and validates the embedded defaults.yaml on
// the first call and caches the result. Subsequent calls return the cached
// slice (or cached error).
func loadCoreTypesRecipeInfo() ([]CoreTypesRecipeInfo, error) {
	coreTypesOnce.Do(func() {
		defaults, err := manifest.ParseDefaults()
		if err != nil {
			coreTypesErr = err
			return
		}
		coreTypesCache, coreTypesErr = buildCoreTypesRecipeInfo(defaults.DefaultRegistration, currentRecipeTag())
	})
	return coreTypesCache, coreTypesErr
}

// buildCoreTypesRecipeInfo validates entries from defaults.yaml and converts
// each one into a CoreTypesRecipeInfo using the path-inference rule. It is a
// pure function (no globals, no I/O) so it is easy to unit-test.
func buildCoreTypesRecipeInfo(entries []string, tag string) ([]CoreTypesRecipeInfo, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("no default resource types are configured in deploy/manifest/defaults.yaml (defaultRegistration is empty); refusing to construct an empty default recipe pack")
	}

	out := make([]CoreTypesRecipeInfo, 0, len(entries))
	for _, entry := range entries {
		location, err := recipeLocationForEntry(entry, tag)
		if err != nil {
			return nil, err
		}
		out = append(out, CoreTypesRecipeInfo{
			ResourceType:   entry,
			RecipeLocation: location,
		})
	}
	return out, nil
}

// recipeLocationForEntry converts a single defaults.yaml entry of the form
// "Radius.<Namespace>/<typeName>" into a published OCI recipe location of the
// form "ghcr.io/radius-project/kube-recipes/<lowercased-typename>:<tag>".
//
// Returns an error that quotes the offending entry literally when the entry
// does not match the expected shape.
func recipeLocationForEntry(entry string, tag string) (string, error) {
	if !strings.HasPrefix(entry, namespacePrefix) {
		return "", fmt.Errorf("malformed defaultRegistration entry %q: namespace must start with %q", entry, namespacePrefix)
	}
	slash := strings.Index(entry, "/")
	if slash < 0 {
		return "", fmt.Errorf("malformed defaultRegistration entry %q: missing '/' separator between namespace and type name", entry)
	}
	namespace := entry[:slash]
	typeName := entry[slash+1:]
	// A bare "Radius." with no following namespace text is malformed.
	if strings.TrimPrefix(namespace, namespacePrefix) == "" {
		return "", fmt.Errorf("malformed defaultRegistration entry %q: namespace after %q must not be empty", entry, namespacePrefix)
	}
	if typeName == "" {
		return "", fmt.Errorf("malformed defaultRegistration entry %q: type name after '/' must not be empty", entry)
	}
	return fmt.Sprintf("%s/%s:%s", recipeRegistryPrefix, strings.ToLower(typeName), tag), nil
}

// currentRecipeTag returns the OCI tag used for default recipes published by
// Radius: the current release channel on stable builds, "latest" on edge.
func currentRecipeTag() string {
	if version.IsEdgeChannel() {
		return "latest"
	}
	return version.Channel()
}
