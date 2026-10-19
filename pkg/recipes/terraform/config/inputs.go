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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RecipeInputsOutputName is reserved for inputs retained in Terraform state for deletion.
const RecipeInputsOutputName = "_radius_recipe_inputs"

// AddRecipeInputs retains module inputs in a sensitive state output for destroy.
func (cfg *TerraformConfig) AddRecipeInputs(moduleName string) error {
	module, ok := cfg.Module[moduleName]
	if !ok {
		return fmt.Errorf("module %q not found in the initialized terraform config", moduleName)
	}
	if _, exists := cfg.Output[RecipeInputsOutputName]; exists {
		return fmt.Errorf("output %q is reserved for Radius recipe inputs", RecipeInputsOutputName)
	}

	inputs := RecipeParams{}
	for name, value := range module {
		if !isModuleMetadata(name) {
			inputs[name] = value
		}
	}
	data, err := json.Marshal(inputs)
	if err != nil {
		return fmt.Errorf("encoding terraform recipe inputs: %w", err)
	}
	if cfg.Output == nil {
		cfg.Output = map[string]any{}
	}
	// Base64 prevents Terraform from evaluating interpolation syntax inside the saved JSON.
	// It is not encryption; the state backend must protect this sensitive output.
	cfg.Output[RecipeInputsOutputName] = map[string]any{
		"value":     base64.StdEncoding.EncodeToString(data),
		"sensitive": true,
	}
	return nil
}

// RestoreRecipeInputs fills missing or null module input values with those saved during deployment.
// Current inputs take precedence, and only arguments in the current config are restored, so the
// config stays valid when the module version changed after deployment. The saved value is also
// kept in the reserved output so that a retry after a failed destroy can restore it again.
// It must only be used for destroy, never for create or update.
func (cfg *TerraformConfig) RestoreRecipeInputs(moduleName, encoded string) error {
	module, ok := cfg.Module[moduleName]
	if !ok {
		return fmt.Errorf("module %q not found in the initialized terraform config", moduleName)
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return errors.New("invalid saved terraform recipe inputs: expected base64")
	}
	var inputs RecipeParams
	if err := decodeJSON(data, &inputs); err != nil || inputs == nil {
		// Do not include the decoder error: it can contain secret input data.
		return errors.New("invalid saved terraform recipe inputs: expected a JSON object")
	}
	for name := range inputs {
		if isModuleMetadata(name) {
			return errors.New("invalid saved terraform recipe inputs: module metadata is not allowed")
		}
	}
	restored := map[string]any{}
	for name, value := range module {
		if isModuleMetadata(name) {
			continue
		}
		// Convert typed values such as the recipe context struct to JSON values so they can be merged.
		normalized, err := normalizeInput(value)
		if err != nil {
			return fmt.Errorf("encoding current terraform recipe input %q: %w", name, err)
		}
		saved, ok := inputs[name]
		if ok {
			normalized = fillMissingValues(normalized, saved)
		}
		restored[name] = normalized
	}
	for name, value := range restored {
		module[name] = value
	}
	if cfg.Output == nil {
		cfg.Output = map[string]any{}
	}
	cfg.Output[RecipeInputsOutputName] = map[string]any{
		"value":     encoded,
		"sensitive": true,
	}
	return nil
}

func normalizeInput(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := decodeJSON(data, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// decodeJSON decodes a single JSON value and keeps numbers as json.Number, so that integers
// larger than 2^53 are not rounded through float64 when the config is written again.
func decodeJSON(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after JSON value")
	}
	return nil
}

// fillMissingValues returns current with null values and missing object keys taken from saved.
// Radius redacts sensitive values to null, so non-null current values are never replaced.
func fillMissingValues(current, saved any) any {
	if current == nil {
		return saved
	}
	switch currentValue := current.(type) {
	case map[string]any:
		savedValue, ok := saved.(map[string]any)
		if !ok {
			return current
		}
		for key, value := range savedValue {
			currentValue[key] = fillMissingValues(currentValue[key], value)
		}
	case []any:
		savedValue, ok := saved.([]any)
		if !ok || len(savedValue) != len(currentValue) {
			return current
		}
		for i := range currentValue {
			currentValue[i] = fillMissingValues(currentValue[i], savedValue[i])
		}
	}
	return current
}

func isModuleMetadata(name string) bool {
	return name == moduleSourceKey || name == moduleVersionKey || name == moduleProvidersKey
}
