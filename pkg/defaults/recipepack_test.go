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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	manifestassets "github.com/radius-project/radius/deploy/manifest"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
)

// TestDefaultKubernetesRecipePack_EmbeddedPack checks the embedded pack without
// listing its types, which come from the pinned upstream revision: it must
// load, and rad must register every type the pack serves.
func TestDefaultKubernetesRecipePack_EmbeddedPack(t *testing.T) {
	t.Parallel()

	pack, err := DefaultKubernetesRecipePack()
	require.NoError(t, err, "embedded Kubernetes recipe pack must load; run make sync-resource-types")
	require.NotEmpty(t, pack.Recipes)

	raw, err := manifestassets.FS.ReadFile(manifestassets.DefaultsYAMLPath)
	require.NoError(t, err)
	var parsed catalog
	require.NoError(t, yaml.Unmarshal(raw, &parsed))

	for resourceType, recipe := range pack.Recipes {
		assert.Contains(t, parsed.DefaultRegistration, resourceType, "recipe pack type must be registered by default")
		require.NotNil(t, recipe.Source, resourceType)
		_, err := RecipeSourceRepository(*recipe.Source)
		assert.NoError(t, err, resourceType)
	}
}

// TestDefaultKubernetesRecipePack_ReturnsCopy guards the read-only contract: a
// caller mutating the result must not change what the next caller sees.
func TestDefaultKubernetesRecipePack_ReturnsCopy(t *testing.T) {
	t.Parallel()

	first, err := DefaultKubernetesRecipePack()
	require.NoError(t, err)
	for _, recipe := range first.Recipes {
		recipe.Source = to.Ptr("mutated")
		recipe.Parameters = map[string]any{"mutated": true}
	}
	first.Recipes["Mutated/type"] = &corerpv20250801.RecipeDefinition{}

	second, err := DefaultKubernetesRecipePack()
	require.NoError(t, err)
	assert.NotContains(t, second.Recipes, "Mutated/type")
	for _, recipe := range second.Recipes {
		assert.NotEqual(t, "mutated", *recipe.Source)
		assert.NotContains(t, recipe.Parameters, "mutated")
	}
}

func TestParseRecipePack(t *testing.T) {
	t.Parallel()

	template := func(packType, properties string) string {
		return `{"imports":{"radius":{"provider":"Radius","version":"latest"}},"resources":{` +
			`"other":{"type":"Radius.Core/environments@2025-08-01-preview"},` +
			`"pack":{"type":"` + packType + `","properties":{"name":"default","properties":` + properties + `}}}}`
	}
	pack := func(recipes string) string {
		return template(recipePackResourceType, `{"recipes":{`+recipes+`}}`)
	}

	tests := []struct {
		name    string
		raw     string
		want    *corerpv20250801.RecipePackProperties
		wantErr string
	}{
		{
			name: "valid",
			raw: pack(`"Radius.Data/redisCaches":{"kind":"bicep","source":"ghcr.io/org/recipes/rediscaches:latest"},` +
				`"Radius.Compute/routes":{"kind":"terraform","source":"localhost:5000/routes:0.1","plainHttp":true,` +
				`"parameters":{"gatewayName":"radius","escaped":"[[literal]","list":[1,"a"]}}`),
			want: &corerpv20250801.RecipePackProperties{
				Recipes: map[string]*corerpv20250801.RecipeDefinition{
					"Radius.Data/redisCaches": {
						Kind:   to.Ptr(corerpv20250801.RecipeKindBicep),
						Source: to.Ptr("ghcr.io/org/recipes/rediscaches:latest"),
					},
					"Radius.Compute/routes": {
						Kind:       to.Ptr(corerpv20250801.RecipeKindTerraform),
						Source:     to.Ptr("localhost:5000/routes:0.1"),
						PlainHTTP:  to.Ptr(true),
						Parameters: map[string]any{"gatewayName": "radius", "escaped": "[[literal]", "list": []any{float64(1), "a"}},
					},
				},
			},
		},
		{name: "invalid JSON", raw: `{`, wantErr: "unexpected end of JSON input"},
		{name: "no pack resource", raw: `{"resources":{}}`, wantErr: "expected exactly one Radius.Core/recipePacks resource, found 0"},
		{
			name:    "two pack resources",
			raw:     `{"resources":{"a":{"type":"Radius.Core/recipePacks@v1"},"b":{"type":"Radius.Core/recipePacks@v1"}}}`,
			wantErr: "found 2",
		},
		{name: "other API version", raw: template("Radius.Core/recipePacks@2030-01-01", `{"recipes":{}}`), wantErr: "rad supports " + recipePackResourceType},
		{name: "no properties", raw: `{"resources":{"pack":{"type":"` + recipePackResourceType + `","properties":{"name":"default"}}}}`, wantErr: "recipe pack has no properties"},
		{name: "no recipes", raw: pack(``), wantErr: "recipe pack has no recipes"},
		{name: "unknown pack field", raw: template(recipePackResourceType, `{"recipes":{},"description":"x"}`), wantErr: `unsupported fields "description"`},
		{name: "read-only pack field", raw: template(recipePackResourceType, `{"recipes":{"A/b":{"kind":"bicep","source":"r/b:1"}},"provisioningState":"Succeeded"}`), wantErr: "read-only"},
		{name: "unknown recipe field", raw: pack(`"A/b":{"kind":"bicep","source":"r/b:1","timeout":5,"retries":1}`), wantErr: `recipe for A/b: unsupported fields "retries", "timeout"`},
		{name: "missing kind", raw: pack(`"A/b":{"source":"r/b:1"}`), wantErr: "recipe for A/b: kind is empty"},
		{name: "unsupported kind", raw: pack(`"A/b":{"kind":"helm","source":"r/b:1"}`), wantErr: `kind "helm" is not supported`},
		{name: "missing source", raw: pack(`"A/b":{"kind":"bicep"}`), wantErr: "source is empty"},
		{name: "untagged source", raw: pack(`"A/b":{"kind":"bicep","source":"localhost:5000/b"}`), wantErr: "has no tag"},
		{name: "digest source", raw: pack(`"A/b":{"kind":"bicep","source":"r/b@sha256:abc"}`), wantErr: "not a digest"},
		{name: "expression source", raw: pack(`"A/b":{"kind":"bicep","source":"[format('r/b:{0}', 'x')]"}`), wantErr: "is an expression"},
		{
			name:    "expression parameter",
			raw:     pack(`"A/b":{"kind":"bicep","source":"r/b:1","parameters":{"p":{"q":["ok","[parameters('x')]"]}}}`),
			wantErr: "parameters.p.q[1] is an expression",
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
			var properties corerpv20250801.RecipePackProperties
			require.NoError(t, properties.UnmarshalJSON(got))
			assert.Equal(t, tt.want, &properties)
		})
	}
}

func TestRecipeSourceRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		source  string
		want    string
		wantErr string
	}{
		{source: "ghcr.io/org/recipes/containers:latest", want: "ghcr.io/org/recipes/containers"},
		{source: "localhost:5000/routes:0.1", want: "localhost:5000/routes"},
		{source: "localhost:5000/routes", wantErr: "has no tag"},
		{source: "r/b@sha256:abc", wantErr: "not a digest"},
		{source: "", wantErr: "source is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			t.Parallel()

			got, err := RecipeSourceRepository(tt.source)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
