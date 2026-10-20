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

package baseresource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// mustLoad returns a freshly loaded base manifest for tests.
func mustLoad(t *testing.T) *BaseManifest {
	t.Helper()
	b, err := Load()
	require.NoError(t, err)
	return b
}

func TestLoad(t *testing.T) {
	b, err := Load()
	require.NoError(t, err)
	require.NotNil(t, b)
}

func TestMustLoad(t *testing.T) {
	require.NotPanics(t, func() { _ = MustLoad() })
}

func TestApply_NilSchema(t *testing.T) {
	require.NoError(t, mustLoad(t).Apply(nil))
}

func TestApply_BareSchemaGetsAllBaseProperties(t *testing.T) {
	b := mustLoad(t)
	schema := map[string]any{"type": "object"}

	require.NoError(t, b.Apply(schema))

	props, err := schemaProperties(schema)
	require.NoError(t, err)

	for _, name := range b.PropertyNames() {
		require.Contains(t, props, name, "base property %q should be merged in", name)
	}

	// environment is the only mandatory base property.
	required, err := toStringSlice(schema["required"])
	require.NoError(t, err)
	require.Equal(t, []string{"environment"}, required)
}

func TestApply_PreservesTypeSpecificProperties(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"size": map[string]any{"type": "integer"},
		},
		"required": []any{"size"},
	}

	require.NoError(t, mustLoad(t).Apply(schema))

	props, err := schemaProperties(schema)
	require.NoError(t, err)
	require.Contains(t, props, "size")
	require.Contains(t, props, "environment")

	// The base "required" is unioned in without dropping the type-specific entry.
	required, err := toStringSlice(schema["required"])
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"size", "environment"}, required)
}

func TestApply_PerTypeWinsOnConflict(t *testing.T) {
	custom := map[string]any{
		"type":        "string",
		"description": "author's own environment shape",
	}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"environment": custom,
		},
	}

	require.NoError(t, mustLoad(t).Apply(schema))

	props, err := schemaProperties(schema)
	require.NoError(t, err)
	require.Equal(t, custom, props["environment"], "author's declaration must not be overwritten")
}

func TestApply_DoesNotDuplicateRequired(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"environment"},
	}

	require.NoError(t, mustLoad(t).Apply(schema))

	required, err := toStringSlice(schema["required"])
	require.NoError(t, err)
	require.Equal(t, []string{"environment"}, required)
}

func TestApply_Idempotent(t *testing.T) {
	b := mustLoad(t)

	first := map[string]any{"type": "object"}
	require.NoError(t, b.Apply(first))

	second := map[string]any{"type": "object"}
	require.NoError(t, b.Apply(second))
	require.NoError(t, b.Apply(second)) // applied twice

	require.Equal(t, first, second, "applying twice must equal applying once")
}

func TestApply_MergedPropertiesAreIndependentCopies(t *testing.T) {
	b := mustLoad(t)
	a := map[string]any{"type": "object"}
	c := map[string]any{"type": "object"}
	require.NoError(t, b.Apply(a))
	require.NoError(t, b.Apply(c))

	aProps, err := schemaProperties(a)
	require.NoError(t, err)
	cProps, err := schemaProperties(c)
	require.NoError(t, err)

	// Mutating one schema's merged property must not affect another's.
	aProps["environment"].(map[string]any)["description"] = "mutated"
	require.NotEqual(t, aProps["environment"], cProps["environment"])
}

func TestApply_InvalidProperties(t *testing.T) {
	schema := map[string]any{"properties": "not-an-object"}
	require.Error(t, mustLoad(t).Apply(schema))
}

func TestApply_InvalidRequired(t *testing.T) {
	schema := map[string]any{"required": "not-a-list"}
	require.Error(t, mustLoad(t).Apply(schema))
}

func TestPropertyNames(t *testing.T) {
	names := mustLoad(t).PropertyNames()
	require.ElementsMatch(t, []string{"application", "environment", "connections", "codeReference"}, names)
	// Sorted for deterministic output.
	require.Equal(t, []string{"application", "codeReference", "connections", "environment"}, names)
}

func TestRequiredNames(t *testing.T) {
	require.Equal(t, []string{"environment"}, mustLoad(t).RequiredNames())
}

func TestConnectionsShapePreserved(t *testing.T) {
	schema := map[string]any{"type": "object"}
	require.NoError(t, mustLoad(t).Apply(schema))

	props, err := schemaProperties(schema)
	require.NoError(t, err)

	connections, err := toStringMap(props["connections"])
	require.NoError(t, err)
	require.Equal(t, "object", connections["type"])

	additional, err := toStringMap(connections["additionalProperties"])
	require.NoError(t, err)
	additionalProps, err := toStringMap(additional["properties"])
	require.NoError(t, err)
	require.Contains(t, additionalProps, "source")
}
