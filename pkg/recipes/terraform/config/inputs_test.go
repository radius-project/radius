/*
Copyright 2026 The Radius Authors.

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
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecipeInputsRoundTrip(t *testing.T) {
	t.Parallel()
	saved := RecipeParams{
		"context": map[string]any{
			"resource": map[string]any{
				"properties": map[string]any{
					"data": map[string]any{
						"password": map[string]any{"value": "secret-${literal}-%{literal}"},
					},
				},
			},
		},
		"enabled": true,
		"counted": float64(2),
		"list":    []any{"first", "second-secret"},
		"null":    nil,
		"removed": "must not be restored",
	}
	cfg := &TerraformConfig{Module: map[string]TFModuleConfig{
		"recipe": {moduleSourceKey: "original", moduleVersionKey: "1.0", moduleProvidersKey: map[string]string{"aws": "aws"}},
	}}
	for name, value := range saved {
		cfg.Module["recipe"][name] = value
	}
	require.NoError(t, cfg.AddRecipeInputs("recipe"))
	output := cfg.Output[RecipeInputsOutputName].(map[string]any)
	require.Equal(t, true, output["sensitive"])
	encoded := output["value"].(string)
	require.NotContains(t, encoded, "secret")
	require.NotContains(t, encoded, "${")
	require.NotContains(t, encoded, "%{")

	// Simulate deletion with a newer module version and redacted sensitive values.
	cfg.Module["recipe"] = TFModuleConfig{
		moduleSourceKey: "current", moduleVersionKey: "2.0",
		moduleProvidersKey: map[string]string{"aws": "aws.current"},
		"context": map[string]any{
			"resource": map[string]any{
				"properties": map[string]any{
					"data":    map[string]any{"password": map[string]any{"value": nil}},
					"current": "kept",
				},
			},
		},
		"enabled":  nil,
		"counted":  float64(3),
		"list":     []any{"first", nil},
		"null":     nil,
		"newInput": "kept",
	}
	cfg.Output[RecipeInputsOutputName] = map[string]any{"value": "redacted", "sensitive": true}
	require.NoError(t, cfg.RestoreRecipeInputs("recipe", encoded))
	require.Equal(t, TFModuleConfig{
		moduleSourceKey: "current", moduleVersionKey: "2.0",
		moduleProvidersKey: map[string]string{"aws": "aws.current"},
		"context": map[string]any{
			"resource": map[string]any{
				"properties": map[string]any{
					"data":    map[string]any{"password": map[string]any{"value": "secret-${literal}-%{literal}"}},
					"current": "kept",
				},
			},
		},
		"enabled":  true,
		"counted":  float64(3),
		"list":     []any{"first", "second-secret"},
		"null":     nil,
		"newInput": "kept",
	}, cfg.Module["recipe"])
	require.Equal(t, map[string]any{"value": encoded, "sensitive": true}, cfg.Output[RecipeInputsOutputName])
}

func TestRecipeInputsRestoreTypedValues(t *testing.T) {
	t.Parallel()
	type secret struct {
		Value any `json:"value"`
	}
	type recipeContext struct {
		Secret secret `json:"secret"`
		Name   string `json:"name"`
	}
	cfg := &TerraformConfig{Module: map[string]TFModuleConfig{
		"recipe": {"context": recipeContext{Secret: secret{Value: "saved"}, Name: "old"}},
	}}
	require.NoError(t, cfg.AddRecipeInputs("recipe"))
	encoded := cfg.Output[RecipeInputsOutputName].(map[string]any)["value"].(string)

	cfg.Module["recipe"]["context"] = &recipeContext{Name: "current"}
	require.NoError(t, cfg.RestoreRecipeInputs("recipe", encoded))
	require.Equal(t, map[string]any{
		"secret": map[string]any{"value": "saved"},
		"name":   "current",
	}, cfg.Module["recipe"]["context"])
}

func TestFillMissingValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		current  any
		saved    any
		expected any
	}{
		{name: "null filled", current: nil, saved: "saved", expected: "saved"},
		{name: "value kept", current: "current", saved: "saved", expected: "current"},
		{name: "missing key filled", current: map[string]any{}, saved: map[string]any{"key": "saved"}, expected: map[string]any{"key": "saved"}},
		{name: "type change kept", current: map[string]any{"key": nil}, saved: "saved", expected: map[string]any{"key": nil}},
		{name: "list length change kept", current: []any{nil}, saved: []any{"one", "two"}, expected: []any{nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, fillMissingValues(tt.current, tt.saved))
		})
	}
}

func TestRecipeInputsRejectInvalidState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		encoded string
	}{
		{name: "not base64", encoded: "invalid secret"},
		{name: "not JSON", encoded: base64.StdEncoding.EncodeToString([]byte("secret"))},
		{name: "null", encoded: base64.StdEncoding.EncodeToString([]byte("null"))},
		{name: "array", encoded: base64.StdEncoding.EncodeToString([]byte("[]"))},
		{name: "source override", encoded: base64.StdEncoding.EncodeToString([]byte(`{"source":"secret"}`))},
		{name: "version override", encoded: base64.StdEncoding.EncodeToString([]byte(`{"version":"secret"}`))},
		{name: "provider override", encoded: base64.StdEncoding.EncodeToString([]byte(`{"providers":"secret"}`))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &TerraformConfig{Module: map[string]TFModuleConfig{"recipe": {"input": "unchanged"}}}
			err := cfg.RestoreRecipeInputs("recipe", tt.encoded)
			require.ErrorContains(t, err, "invalid saved terraform recipe inputs")
			require.NotContains(t, err.Error(), "secret")
			require.Equal(t, TFModuleConfig{"input": "unchanged"}, cfg.Module["recipe"])
		})
	}
}

func TestRecipeInputsRequireModuleAndReservedOutput(t *testing.T) {
	t.Parallel()
	cfg := &TerraformConfig{Module: map[string]TFModuleConfig{"recipe": {}}}
	require.ErrorContains(t, cfg.AddRecipeInputs("missing"), `module "missing" not found`)
	require.ErrorContains(t, cfg.RestoreRecipeInputs("missing", ""), `module "missing" not found`)
	cfg.Output = map[string]any{RecipeInputsOutputName: map[string]any{"value": "user output"}}
	require.ErrorContains(t, cfg.AddRecipeInputs("recipe"), "reserved")
	require.Equal(t, map[string]any{"value": "user output"}, cfg.Output[RecipeInputsOutputName])
}
