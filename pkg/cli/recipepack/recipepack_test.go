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

	"github.com/radius-project/radius/deploy/manifest"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/require"
)

// Test_GetCoreTypesRecipeInfo_MatchesDefaultsYAML asserts that the slice returned
// by GetCoreTypesRecipeInfo is derived from deploy/manifest/defaults.yaml: every
// entry in defaults.yaml appears exactly once, and the count matches.
func Test_GetCoreTypesRecipeInfo_MatchesDefaultsYAML(t *testing.T) {
	defaults, err := manifest.ParseDefaults()
	require.NoError(t, err)
	require.NotEmpty(t, defaults.DefaultRegistration, "defaults.yaml must list at least one default resource type")

	definitions := GetCoreTypesRecipeInfo()
	require.Len(t, definitions, len(defaults.DefaultRegistration),
		"expected one CoreTypesRecipeInfo per defaultRegistration entry")

	actual := make([]string, len(definitions))
	for i, def := range definitions {
		actual[i] = def.ResourceType
		require.NotEmpty(t, def.RecipeLocation, "RecipeLocation should not be empty for %s", def.ResourceType)
	}
	require.ElementsMatch(t, defaults.DefaultRegistration, actual)
}

// Test_GetCoreTypesRecipeInfo_UsesLatestTagForEdgeChannel asserts that on the
// default (edge) build channel every recipe location is tagged :latest.
func Test_GetCoreTypesRecipeInfo_UsesLatestTagForEdgeChannel(t *testing.T) {
	// The test binary is built without ldflags, so channel defaults to "edge".
	require.True(t, version.IsEdgeChannel(), "default should be on edge channel")

	for _, def := range GetCoreTypesRecipeInfo() {
		require.True(t, strings.HasSuffix(def.RecipeLocation, ":latest"),
			"Expected :latest tag for edge channel, got %s", def.RecipeLocation)
	}
}

// Test_NewDefaultRecipePackResource asserts that the constructed RecipePackResource
// contains a Bicep recipe for every entry of defaults.yaml at the inferred location.
func Test_NewDefaultRecipePackResource(t *testing.T) {
	resource := NewDefaultRecipePackResource()

	require.NotNil(t, resource.Location)
	require.Equal(t, "global", *resource.Location)

	require.NotNil(t, resource.Properties)
	require.NotNil(t, resource.Properties.Recipes)

	definitions := GetCoreTypesRecipeInfo()
	require.Len(t, resource.Properties.Recipes, len(definitions))

	for _, def := range definitions {
		recipe, exists := resource.Properties.Recipes[def.ResourceType]
		require.True(t, exists, "Expected recipe for resource type %s to exist", def.ResourceType)
		require.NotNil(t, recipe.RecipeKind)
		require.Equal(t, corerpv20250801.RecipeKindBicep, *recipe.RecipeKind)
		require.NotNil(t, recipe.RecipeLocation)
		require.Equal(t, def.RecipeLocation, *recipe.RecipeLocation)
	}
}

// Test_RecipeLocationForEntry_PathInference asserts the inference rule
// "Radius.<Namespace>/<typeName>" → "<prefix>/<lowercased-typename>:<tag>"
// for each entry currently in defaults.yaml plus a hypothetical mixed-case type.
func Test_RecipeLocationForEntry_PathInference(t *testing.T) {
	const tag = "0.42"

	cases := []struct {
		entry    string
		expected string
	}{
		{"Radius.Compute/containers", "ghcr.io/radius-project/kube-recipes/containers:0.42"},
		{"Radius.Compute/persistentVolumes", "ghcr.io/radius-project/kube-recipes/persistentvolumes:0.42"},
		{"Radius.Compute/routes", "ghcr.io/radius-project/kube-recipes/routes:0.42"},
		{"Radius.Security/secrets", "ghcr.io/radius-project/kube-recipes/secrets:0.42"},
		{"Radius.Data/mySqlDatabases", "ghcr.io/radius-project/kube-recipes/mysqldatabases:0.42"},
		// Hypothetical future type — confirms the rule generalises.
		{"Radius.Demo/Widgets", "ghcr.io/radius-project/kube-recipes/widgets:0.42"},
	}

	for _, tc := range cases {
		t.Run(tc.entry, func(t *testing.T) {
			got, err := recipeLocationForEntry(tc.entry, tag)
			require.NoError(t, err)
			require.Equal(t, tc.expected, got)
		})
	}
}

// Test_RecipeLocationForEntry_RejectsMalformedEntries asserts that every shape
// of malformed entry surfaces an error that quotes the offending entry literally.
func Test_RecipeLocationForEntry_RejectsMalformedEntries(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"missing Radius. prefix", "Microsoft.Compute/containers"},
		{"missing slash separator", "Radius.Compute.containers"},
		{"empty namespace after prefix", "Radius./containers"},
		{"empty type name", "Radius.Compute/"},
		{"empty string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := recipeLocationForEntry(tc.entry, "0.42")
			require.Error(t, err)
			require.Contains(t, err.Error(), `"`+tc.entry+`"`,
				"error %q should quote offending entry %q literally", err.Error(), tc.entry)
		})
	}
}

// Test_BuildCoreTypesRecipeInfo_EmptyEntriesRejected: an empty defaultRegistration
// list must fail rather than produce an empty recipe pack.
func Test_BuildCoreTypesRecipeInfo_EmptyEntriesRejected(t *testing.T) {
	got, err := buildCoreTypesRecipeInfo(nil, "0.42")
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "defaultRegistration is empty")

	got, err = buildCoreTypesRecipeInfo([]string{}, "0.42")
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), "defaultRegistration is empty")
}

// Test_BuildCoreTypesRecipeInfo_PropagatesMalformedEntry asserts that a bad
// entry in the middle of an otherwise valid list aborts construction.
func Test_BuildCoreTypesRecipeInfo_PropagatesMalformedEntry(t *testing.T) {
	entries := []string{
		"Radius.Compute/containers",
		"Radius.Compute/", // malformed: empty type name
		"Radius.Security/secrets",
	}
	got, err := buildCoreTypesRecipeInfo(entries, "0.42")
	require.Error(t, err)
	require.Nil(t, got)
	require.Contains(t, err.Error(), `"Radius.Compute/"`)
}

// Test_DefaultRecipePackID asserts the formatted resource ID stays stable.
func Test_DefaultRecipePackID(t *testing.T) {
	require.Equal(t,
		"/planes/radius/local/resourceGroups/default/providers/Radius.Core/recipePacks/default",
		DefaultRecipePackID())
}
