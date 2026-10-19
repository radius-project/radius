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
	"encoding/json"
	"errors"
	"fmt"
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

// RestoreRecipeInputs replaces module inputs with those saved during deployment.
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
	if err := json.Unmarshal(data, &inputs); err != nil || inputs == nil {
		// Do not include the decoder error: it can contain secret input data.
		return errors.New("invalid saved terraform recipe inputs: expected a JSON object")
	}
	for name := range inputs {
		if isModuleMetadata(name) {
			return errors.New("invalid saved terraform recipe inputs: module metadata is not allowed")
		}
	}
	for name := range module {
		if !isModuleMetadata(name) {
			delete(module, name)
		}
	}
	for name, value := range inputs {
		module[name] = value
	}
	return nil
}

func isModuleMetadata(name string) bool {
	return name == moduleSourceKey || name == moduleVersionKey || name == moduleProvidersKey
}
