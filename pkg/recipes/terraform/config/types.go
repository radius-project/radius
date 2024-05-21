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
	// Terraform represents number of settings related to Terraform's behavior.
	Terraform *TerraformDefinition `json:"terraform"`

	// Provider represents the the configuration for Terraform providers.
	// The key of the map is a string that represents the name of the provider.
	// The value is a slice of maps, where each map represents a specific configuration for the provider.
	// Each configuration map has string keys and values of any type.
	// This structure allows for multiple configurations per provider.
	//
	// For example:
	// {
	//   "aws": [
	//     {
	//       "region": "us-west-2",
	//       "version": "3.0"
	//     },
	//     {
	//       "alias": "east",
	//       "region": "us-east-1"
	//     }
	//   ],
	//   "azurerm": [
	//     {
	//       "tenant": "my-tenantId",
	//       "subscription_id": "my-subscriptionId"
	//     }
	//   ]
	// }
	//
	// In this example, there are two providers: "aws" and "azurerm".
	// The "aws" provider has two configurations: one for the "us-west-2" region and another for the "us-east-1" region with an alias "east".
	// The "azurerm" provider has one configuration.
	//
	// For more information on Terraform provider configuration, refer to:
	// https://developer.hashicorp.com/terraform/language/providers/configuration
	// https://developer.hashicorp.com/terraform/language/syntax/json#provider-blocks
	Provider map[string][]map[string]any `json:"provider,omitempty"`

	// Module is the Terraform module configuration.
	// https://developer.hashicorp.com/terraform/language/modules/syntax
	Module map[string]TFModuleConfig `json:"module"`

	// Output is the Terraform output configuration.
	// https://developer.hashicorp.com/terraform/language/values/outputs
	Output map[string]any `json:"output,omitempty"`
}

type TerraformDefinition struct {
	// Backend defines where Terraform stores its state.
	// https://developer.hashicorp.com/terraform/language/state
	Backend map[string]interface{} `json:"backend"`

	// RequiredProviders is the list of required Terraform providers.
	// The json serialised json field name reflects the fieldname "required_providers" expected
	// in the terraform configuration file.
	// ref: https://developer.hashicorp.com/terraform/language/providers/configuration
	RequiredProviders map[string]*RequiredProviderInfo `json:"required_providers,omitempty"`
}

// RequiredProviderInfo represents details for a provider listed under the required_providers block in a Terraform module.
// The json serialised json field names reflect the fieldnames expected in the terraform configuration file.
// ref: https://developer.hashicorp.com/terraform/language/providers/configuration
type RequiredProviderInfo struct {
	Source               string   `json:"source,omitempty"`                // The source of the provider.
	Version              string   `json:"version,omitempty"`               // The version of the provider.
	ConfigurationAliases []string `json:"configuration_aliases,omitempty"` // The configuration aliases for the provider.
}
