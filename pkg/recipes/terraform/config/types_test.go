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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetParams(t *testing.T) {
	c := TFModuleConfig{}

	c.SetParams(RecipeParams{
		"foo": map[string]any{
			"bar": "baz",
		},
		"bar": map[string]any{
			"baz": "foo",
		},
	})

	require.Equal(t, 2, len(c))
	require.Equal(t, c["foo"].(map[string]any), map[string]any{"bar": "baz"})
	require.Equal(t, c["bar"].(map[string]any), map[string]any{"baz": "foo"})
}

func TestSetParams_EmptyMapString(t *testing.T) {
	c := TFModuleConfig{}

	c.SetParams(RecipeParams{
		"tags":        "{}", // This should be converted to an empty map
		"normalParam": "value",
		"existingMap": map[string]any{
			"key": "value",
		},
		"emptyMap": map[string]any{}, // This should remain as is
	})

	require.Equal(t, 4, len(c))

	// Verify that the string "{}" was converted to an empty map
	tags, ok := c["tags"].(map[string]any)
	require.True(t, ok, "tags should be a map[string]any")
	require.Equal(t, map[string]any{}, tags)

	// Verify other parameters remain unchanged
	require.Equal(t, "value", c["normalParam"])
	require.Equal(t, map[string]any{"key": "value"}, c["existingMap"])
	require.Equal(t, map[string]any{}, c["emptyMap"])
}

func TestSetParams_WhitespaceHandling(t *testing.T) {
	c := TFModuleConfig{}

	c.SetParams(RecipeParams{
		"tags1": "{ }",     // With space inside
		"tags2": "{}\n",    // With newline
		"tags3": " {} ",    // With surrounding spaces
		"array1": "[]",     // Empty array
		"array2": "[ ]",    // Array with space inside
		"array3": "[]\n",   // Array with newline
	})

	// Verify empty maps (all variations should be converted)
	for _, key := range []string{"tags1", "tags2", "tags3"} {
		val, ok := c[key].(map[string]any)
		require.True(t, ok, "%s should be a map[string]any", key)
		require.Equal(t, map[string]any{}, val, "%s should be an empty map", key)
	}

	// Verify empty arrays (all variations should be converted)
	for _, key := range []string{"array1", "array2", "array3"} {
		val, ok := c[key].([]any)
		require.True(t, ok, "%s should be a []any", key)
		require.Equal(t, []any{}, val, "%s should be an empty array", key)
	}
}

func TestSetParams_NestedStructures(t *testing.T) {
	c := TFModuleConfig{}

	c.SetParams(RecipeParams{
		"config": map[string]any{
			"tags":     "{}",
			"metadata": "{}",
			"items":    "[]",
			"nested": map[string]any{
				"deep": map[string]any{
					"tags": "{}",
				},
			},
		},
		"list": []any{
			"{}",
			"[]",
			map[string]any{
				"tags": "{}",
			},
		},
	})

	// Verify nested map normalization
	config := c["config"].(map[string]any)
	require.Equal(t, map[string]any{}, config["tags"])
	require.Equal(t, map[string]any{}, config["metadata"])
	require.Equal(t, []any{}, config["items"])
	
	// Verify deep nested normalization
	nested := config["nested"].(map[string]any)
	deep := nested["deep"].(map[string]any)
	require.Equal(t, map[string]any{}, deep["tags"])

	// Verify array normalization
	list := c["list"].([]any)
	require.Equal(t, map[string]any{}, list[0])
	require.Equal(t, []any{}, list[1])
	nestedInList := list[2].(map[string]any)
	require.Equal(t, map[string]any{}, nestedInList["tags"])
}

func TestSetParams_PreservesNonEmptyStrings(t *testing.T) {
	c := TFModuleConfig{}

	c.SetParams(RecipeParams{
		"notEmpty1": "{\"key\": \"value\"}",
		"notEmpty2": "[1, 2, 3]",
		"notEmpty3": "{ key: value }",
		"normalStr": "just a string",
	})

	// All these should remain as strings since they're not empty
	require.Equal(t, "{\"key\": \"value\"}", c["notEmpty1"])
	require.Equal(t, "[1, 2, 3]", c["notEmpty2"])
	require.Equal(t, "{ key: value }", c["notEmpty3"])
	require.Equal(t, "just a string", c["normalStr"])
}
