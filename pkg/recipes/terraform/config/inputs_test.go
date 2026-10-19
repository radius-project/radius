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
	inputs := RecipeParams{
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
		"list":    []any{"first", nil},
		"null":    nil,
	}
	cfg := &TerraformConfig{Module: map[string]TFModuleConfig{
		"recipe": {moduleSourceKey: "original", moduleVersionKey: "1.0", moduleProvidersKey: map[string]string{"aws": "aws"}},
	}}
	// Include an explicit null to verify that restoration preserves the saved input exactly.
	for name, value := range inputs {
		cfg.Module["recipe"][name] = value
	}
	require.NoError(t, cfg.AddRecipeInputs("recipe"))
	output := cfg.Output[RecipeInputsOutputName].(map[string]any)
	require.Equal(t, true, output["sensitive"])
	encoded := output["value"].(string)
	require.NotContains(t, encoded, "secret")
	require.NotContains(t, encoded, "${")
	require.NotContains(t, encoded, "%{")

	cfg.Module["recipe"] = TFModuleConfig{
		moduleSourceKey: "current", moduleVersionKey: "2.0",
		moduleProvidersKey: map[string]string{"aws": "aws.current"},
		"context":          nil,
		"newInput":         "must not survive",
	}
	require.NoError(t, cfg.RestoreRecipeInputs("recipe", encoded))
	expected := TFModuleConfig{
		moduleSourceKey: "current", moduleVersionKey: "2.0",
		moduleProvidersKey: map[string]string{"aws": "aws.current"},
	}
	for name, value := range inputs {
		expected[name] = value
	}
	require.Equal(t, expected, cfg.Module["recipe"])
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
