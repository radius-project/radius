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
	// ModuleSourceKey represents the key for the module source parameter.
	ModuleSourceKey = "source"
	// ModuleVersionKey represents the key for the module version parameter.
	ModuleVersionKey = "version"
	// ModuleRecipeContextKey represents the key for the module recipe context object parameter.
	ModuleRecipeContextKey = "context"

	mainConfigFileName = "main.tf.json"
)

// TFMoudleConfig is the type of Terraform module configuration.
type TFModuleConfig map[string]any

// RecipeParams is the type of recipe parameter map.
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
	Provider map[string]any `json:"provider,omitempty"`

	// Module is the Terraform module configuration.
	Module map[string]TFModuleConfig `json:"module"`

	// moduleName is the internal property to identity the module name in the configuration. This isn't marshalled to JSON.
	moduleName string `json:"-"`

	// workingDir is the internal property to represent the working directory of the module. This isn't marshalled to JSON.
	workingDir string `json:"-"`
}
