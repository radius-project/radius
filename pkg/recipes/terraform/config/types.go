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

const (
	// moduleSourceKey represents the key for the module source parameter.
	moduleSourceKey = "source"
	// moduleVersionKey represents the key for the module version parameter.
	moduleVersionKey = "version"

	mainConfigFileName = "main.tf.json"
)

// TFModuleConfig is the type of Terraform module configuration.
type TFModuleConfig map[string]any

// RecipeParams is the map of recipe parameters and its values.
type RecipeParams map[string]any

// SetParams sets the recipe parameters in the Terraform module configuration.
func (tf TFModuleConfig) SetParams(params RecipeParams) {
	for k, v := range params {
		tf[k] = v
	}
}

// TerraformConfig represents the Terraform configuration file structure for properties populated in the configuration by Radius.
type TerraformConfig struct {
	// Provider is the Terraform provider configuration.
	// https://developer.hashicorp.com/terraform/language/providers/configuration
	Provider map[string]any `json:"provider,omitempty"`

	// Module is the Terraform module configuration.
	// https://developer.hashicorp.com/terraform/language/modules/syntax
	Module map[string]TFModuleConfig `json:"module"`

	// Output is the Terraform output configuration.
	// https://developer.hashicorp.com/terraform/language/values/outputs
	Output map[string]any `json:"output,omitempty"`
}
