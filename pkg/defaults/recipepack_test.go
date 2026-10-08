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

package defaults

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	manifestassets "github.com/radius-project/radius/deploy/manifest"
)

// TestDefaultKubernetesRecipes_EmbeddedPack checks the embedded pack without
// listing its types, which come from the pinned upstream revision: it must
// load, every recipe must be usable by rad, and rad must register every type
// the pack serves.
func TestDefaultKubernetesRecipes_EmbeddedPack(t *testing.T) {
	t.Parallel()

	recipes := DefaultKubernetesRecipes()
	require.NotEmpty(t, recipes, "embedded Kubernetes recipe pack must load; run make sync-resource-types")

	raw, err := manifestassets.FS.ReadFile(manifestassets.DefaultsYAMLPath)
	require.NoError(t, err)
	var parsed catalog
	require.NoError(t, yaml.Unmarshal(raw, &parsed))

	for _, recipe := range recipes {
		assert.Contains(t, parsed.DefaultRegistration, recipe.ResourceType, "recipe pack type must be registered by default")
		assert.NotEmpty(t, recipe.Kind, recipe.ResourceType)
		assert.NotEmpty(t, recipe.Image, recipe.ResourceType)
		assert.NotContains(t, recipe.Image[strings.LastIndex(recipe.Image, "/")+1:], ":", "image must not carry a tag")
	}
	assert.True(t, slices.IsSortedFunc(recipes, func(a, b RecipePackRecipe) int {
		return strings.Compare(a.ResourceType, b.ResourceType)
	}))
}

// TestDefaultKubernetesRecipes_ReturnsCopy guards the read-only contract: a
// caller mutating the result must not change what the next caller sees.
func TestDefaultKubernetesRecipes_ReturnsCopy(t *testing.T) {
	t.Parallel()

	first := DefaultKubernetesRecipes()
	require.NotEmpty(t, first)
	first[0].Image = "mutated"
	for i := range first {
		if first[i].Parameters != nil {
			first[i].Parameters["mutated"] = true
		}
	}

	for _, recipe := range DefaultKubernetesRecipes() {
		assert.NotEqual(t, "mutated", recipe.Image)
		assert.NotContains(t, recipe.Parameters, "mutated")
	}
}

func TestParseRecipePack(t *testing.T) {
	t.Parallel()

	pack := func(recipes string) string {
		return `{"resources":{"other":{"type":"Radius.Core/environments@2025-08-01-preview"},` +
			`"pack":{"type":"Radius.Core/recipePacks@2025-08-01-preview","properties":{"name":"default","properties":{"recipes":{` +
			recipes + `}}}}}}`
	}

	tests := []struct {
		name    string
		raw     string
		want    []RecipePackRecipe
		wantErr string
	}{
		{
			name: "valid",
			raw: pack(`"Radius.Data/redisCaches":{"kind":"bicep","source":"ghcr.io/org/recipes/rediscaches:latest"},` +
				`"Radius.Compute/routes":{"kind":"bicep","source":"localhost:5000/routes:0.1","parameters":{"gatewayName":"radius"}}`),
			want: []RecipePackRecipe{
				{ResourceType: "Radius.Compute/routes", Kind: "bicep", Image: "localhost:5000/routes", Parameters: map[string]any{"gatewayName": "radius"}},
				{ResourceType: "Radius.Data/redisCaches", Kind: "bicep", Image: "ghcr.io/org/recipes/rediscaches"},
			},
		},
		{name: "invalid JSON", raw: `{`, wantErr: "unexpected end of JSON input"},
		{name: "no pack resource", raw: `{"resources":{}}`, wantErr: "expected exactly one Radius.Core/recipePacks resource, found 0"},
		{
			name:    "two pack resources",
			raw:     `{"resources":{"a":{"type":"Radius.Core/recipePacks@v1"},"b":{"type":"Radius.Core/recipePacks@v1"}}}`,
			wantErr: "found 2",
		},
		{name: "no recipes", raw: pack(``), wantErr: "recipe pack has no recipes"},
		{name: "missing kind", raw: pack(`"A/b":{"source":"r/b:1"}`), wantErr: "recipe for A/b has no kind"},
		{name: "missing source", raw: pack(`"A/b":{"kind":"bicep"}`), wantErr: "source is empty"},
		{name: "untagged source", raw: pack(`"A/b":{"kind":"bicep","source":"localhost:5000/b"}`), wantErr: "has no tag"},
		{name: "digest source", raw: pack(`"A/b":{"kind":"bicep","source":"r/b@sha256:abc"}`), wantErr: "not a digest"},
		{name: "expression source", raw: pack(`"A/b":{"kind":"bicep","source":"[format('r/b:{0}', 'x')]"}`), wantErr: "is an expression"},
		{
			name:    "expression parameter",
			raw:     pack(`"A/b":{"kind":"bicep","source":"r/b:1","parameters":{"p":"[parameters('x')]"}}`),
			wantErr: `parameter "p" is an expression`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRecipePack([]byte(tt.raw))
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
