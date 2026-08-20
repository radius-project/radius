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
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/defaults"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

func Test_GetDefaultRecipePackDefinition(t *testing.T) {
	definitions := GetCoreTypesRecipeInfo()

	// Verify we have the expected number of definitions
	require.Len(t, definitions, 6)

	// Verify expected resource types
	expectedResourceTypes := []string{
		"Radius.Compute/containers",
		"Radius.Compute/persistentVolumes",
		"Radius.Compute/routes",
		"Radius.Security/secrets",
		"Radius.Data/mySqlDatabases",
		"Radius.Data/postgreSqlDatabases",
	}
	actualResourceTypes := make([]string, len(definitions))
	for i, def := range definitions {
		actualResourceTypes[i] = def.ResourceType
		require.NotEmpty(t, def.Source, "Source should not be empty for %s", def.ResourceType)
	}
	require.ElementsMatch(t, expectedResourceTypes, actualResourceTypes)
}

func Test_GetDefaultRecipePackDefinition_UsesEdgeTagForEdgeChannel(t *testing.T) {
	// The test binary is built without ldflags, so channel defaults to "edge".
	require.True(t, version.IsEdgeChannel(), "default should be on edge channel")

	definitions := GetCoreTypesRecipeInfo()
	for _, def := range definitions {
		require.True(t, strings.HasSuffix(def.Source, ":edge"),
			"Expected :edge tag for edge channel, got %s", def.Source)
	}
}

func Test_resolveRecipeTag(t *testing.T) {
	// Kept in sync with deploy/manifest/defaults.yaml by `make update-resource-types`.
	pin, ok := defaults.ResourceTypePin("Radius.Compute")
	require.True(t, ok, "Radius.Compute must be pinned in defaults.yaml")
	require.NotEmpty(t, pin.Ref)

	testcases := []struct {
		name         string
		resourceType string
		isEdge       bool
		expected     string
	}{
		{
			name:         "edge channel uses mutable tag",
			resourceType: "Radius.Compute/containers",
			isEdge:       true,
			expected:     "edge",
		},
		{
			name:         "release channel uses pinned namespace ref",
			resourceType: "Radius.Compute/containers",
			isEdge:       false,
			expected:     pin.Ref,
		},
		{
			name:         "release channel falls back on malformed resource type",
			resourceType: "NotAResourceType",
			isEdge:       false,
			expected:     "edge",
		},
		{
			name:         "release channel falls back on unpinned namespace",
			resourceType: "Contoso.Example/widgets",
			isEdge:       false,
			expected:     "edge",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, resolveRecipeTag(tc.resourceType, tc.isEdge))
		})
	}
}

func Test_NewDefaultRecipePackResource(t *testing.T) {
	resource := NewDefaultRecipePackResource()

	// Verify location
	require.NotNil(t, resource.Location)
	require.Equal(t, "global", *resource.Location)

	// Verify properties exist
	require.NotNil(t, resource.Properties)
	require.NotNil(t, resource.Properties.Recipes)

	// Verify the resource contains recipes for all core types.
	definitions := GetCoreTypesRecipeInfo()
	require.Len(t, resource.Properties.Recipes, len(definitions))

	for _, def := range definitions {
		recipe, exists := resource.Properties.Recipes[def.ResourceType]
		require.True(t, exists, "Expected recipe for resource type %s to exist", def.ResourceType)
		require.NotNil(t, recipe.Kind)
		require.Equal(t, corerpv20250801.RecipeKindBicep, *recipe.Kind)
		require.NotNil(t, recipe.Source)
		require.Equal(t, def.Source, *recipe.Source)
		require.Equal(t, def.Parameters, recipe.Parameters)
	}

	require.Contains(t, resource.Properties.Recipes, "Radius.Compute/routes")
	routeRecipe := resource.Properties.Recipes["Radius.Compute/routes"]
	require.NotNil(t, routeRecipe)
	require.Equal(t, map[string]any{
		"gatewayName":      DefaultRoutesGatewayName,
		"gatewayNamespace": DefaultRoutesGatewayNamespace,
	}, routeRecipe.Parameters)

	require.Contains(t, resource.Properties.Recipes, "Radius.Data/postgreSqlDatabases")
	postgreSQLRecipe := resource.Properties.Recipes["Radius.Data/postgreSqlDatabases"]
	require.NotNil(t, postgreSQLRecipe)
	require.Equal(t, "ghcr.io/radius-project/kube-recipes/postgresqldatabases:edge", *postgreSQLRecipe.Source)
}

func Test_NormalizeRecipePacks(t *testing.T) {
	testcases := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "single value",
			input:    []string{"pack1"},
			expected: []string{"pack1"},
		},
		{
			name:     "comma-separated values",
			input:    []string{"pack1,pack2,pack3"},
			expected: []string{"pack1", "pack2", "pack3"},
		},
		{
			name:     "trims whitespace",
			input:    []string{" pack1 , pack2 ,  pack3"},
			expected: []string{"pack1", "pack2", "pack3"},
		},
		{
			name:     "drops empty entries",
			input:    []string{"pack1,,pack2", "", " , "},
			expected: []string{"pack1", "pack2"},
		},
		{
			name:     "deduplicates repeated flags",
			input:    []string{"pack1", "pack1"},
			expected: []string{"pack1"},
		},
		{
			name:     "deduplicates within comma list",
			input:    []string{"pack1,pack1,pack2"},
			expected: []string{"pack1", "pack2"},
		},
		{
			name:     "deduplicates across mixed sources preserving order",
			input:    []string{"pack2", "pack1,pack2", " pack1 ", "pack3"},
			expected: []string{"pack2", "pack1", "pack3"},
		},
		{
			name:     "treats whitespace-only difference as duplicate",
			input:    []string{"pack1", " pack1 "},
			expected: []string{"pack1"},
		},
		{
			name:     "preserves full resource ID and dedupes",
			input:    []string{"/planes/radius/local/resourcegroups/g/providers/Radius.Core/recipePacks/p1,/planes/radius/local/resourcegroups/g/providers/Radius.Core/recipePacks/p1"},
			expected: []string{"/planes/radius/local/resourcegroups/g/providers/Radius.Core/recipePacks/p1"},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, NormalizeRecipePacks(tc.input))
		})
	}
}

func Test_RefExists(t *testing.T) {
	env1 := "/planes/radius/local/resourceGroups/g/providers/Radius.Core/environments/env1"
	env2 := "/planes/radius/local/resourceGroups/g/providers/Radius.Core/environments/env2"

	testcases := []struct {
		name     string
		refs     []*string
		id       string
		expected bool
	}{
		{name: "nil list", refs: nil, id: env1, expected: false},
		{name: "present", refs: []*string{to.Ptr(env1), to.Ptr(env2)}, id: env2, expected: true},
		{name: "absent", refs: []*string{to.Ptr(env1)}, id: env2, expected: false},
		{name: "ignores nil entries", refs: []*string{nil, to.Ptr(env1)}, id: env1, expected: true},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, RefExists(tc.refs, tc.id))
		})
	}
}

func Test_ResolveID(t *testing.T) {
	const scope = "/planes/radius/local/resourceGroups/test-group"

	tests := []struct {
		name           string
		recipePack     string
		expectedID     string
		expectedFullID bool
	}{
		{
			name:       "bare name is scoped to the workspace resource group",
			recipePack: "my-pack",
			expectedID: scope + "/providers/Radius.Core/recipePacks/my-pack",
		},
		{
			name:           "full resource ID is used as-is",
			recipePack:     "/planes/radius/local/resourceGroups/other-group/providers/Radius.Core/recipePacks/my-pack",
			expectedID:     "/planes/radius/local/resourceGroups/other-group/providers/Radius.Core/recipePacks/my-pack",
			expectedFullID: true,
		},
		{
			name:           "trailing slash on a full resource ID is normalized",
			recipePack:     "/planes/radius/local/resourceGroups/other-group/providers/Radius.Core/recipePacks/my-pack/",
			expectedID:     "/planes/radius/local/resourceGroups/other-group/providers/Radius.Core/recipePacks/my-pack",
			expectedFullID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id, isFullID, err := ResolveID(tt.recipePack, scope)
			require.NoError(t, err)
			require.Equal(t, tt.expectedID, id.String())
			require.Equal(t, tt.expectedFullID, isFullID)
		})
	}

	t.Run("returns an error for an invalid workspace scope", func(t *testing.T) {
		t.Parallel()

		_, _, err := ResolveID("my-pack", "not-a-scope")
		require.Error(t, err)
	})

	rejected := []struct {
		name       string
		recipePack string
	}{
		{
			name:       "resource ID of another type",
			recipePack: "/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/environments/my-env",
		},
		{
			name:       "resource group scope",
			recipePack: "/planes/radius/local/resourceGroups/test-group",
		},
		{
			name:       "plane scope",
			recipePack: "/planes/radius/local",
		},
	}

	for _, tt := range rejected {
		t.Run("rejects "+tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := ResolveID(tt.recipePack, scope)
			require.Error(t, err)
			require.True(t, clierrors.IsFriendlyError(err))
			require.Contains(t, err.Error(), "is not a recipe pack resource ID")
		})
	}
}

func Test_NotFoundError(t *testing.T) {
	t.Run("bare name names the resource group and shows the full ID form", func(t *testing.T) {
		t.Parallel()

		id, isFullID, err := ResolveID("my-pack", "/planes/radius/local/resourceGroups/test-group")
		require.NoError(t, err)

		require.Equal(t,
			`Recipe pack "my-pack" does not exist in resource group "test-group". To reference a recipe pack in another resource group, pass its full resource ID, for example: /planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/my-pack`,
			NotFoundError("my-pack", id, isFullID).Error())
	})

	t.Run("full resource ID omits the cross-group hint", func(t *testing.T) {
		t.Parallel()

		packID := "/planes/radius/local/resourceGroups/other-group/providers/Radius.Core/recipePacks/my-pack"
		id, isFullID, err := ResolveID(packID, "/planes/radius/local/resourceGroups/test-group")
		require.NoError(t, err)

		require.Equal(t,
			`Recipe pack "`+packID+`" does not exist. Please provide a valid recipe pack to set on the environment.`,
			NotFoundError(packID, id, isFullID).Error())
	})
}
