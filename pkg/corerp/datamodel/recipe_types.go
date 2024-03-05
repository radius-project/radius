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

package datamodel

// RecipeConfigProperties - Configuration for Recipes. Defines how each type of Recipe should be configured and run.
type RecipeConfigProperties struct {
	// Configuration for Terraform Recipes. Controls how Terraform plans and applies templates as part of Recipe deployment.
	Terraform TerraformConfigProperties `json:"terraform,omitempty"`

	// Env specifies the environment variables to be set during the Terraform Recipe execution.
	Env EnvironmentVariables `json:"env,omitempty"`
}

// TerraformConfigProperties - Configuration for Terraform Recipes. Controls how Terraform plans and applies templates as
// part of Recipe deployment.
type TerraformConfigProperties struct {
	// Authentication information used to access private Terraform module sources. Supported module sources: Git.
	Authentication AuthConfig `json:"authentication,omitempty"`

	// Providers specifies the Terraform provider configurations. Controls how Terraform interacts with cloud providers, SaaS providers, and other APIs: https://developer.hashicorp.com/terraform/language/providers/configuration.// Providers specifies the Terraform provider configurations.
	Providers map[string][]ProviderConfigProperties `json:"providers,omitempty"`
}

// AuthConfig - Authentication information used to access private Terraform module sources. Supported module sources: Git.
type AuthConfig struct {
	// Authentication information used to access private Terraform modules from Git repository sources.
	Git GitAuthConfig `json:"git,omitempty"`
}

// GitAuthConfig - Authentication information used to access private Terraform modules from Git repository sources.
type GitAuthConfig struct {
	// Personal Access Token (PAT) configuration used to authenticate to Git platforms.
	PAT map[string]SecretConfig `json:"pat,omitempty"`
}

// SecretConfig - Personal Access Token (PAT) configuration used to authenticate to Git platforms.
type SecretConfig struct {
	// The ID of an Applications.Core/SecretStore resource containing the Git platform personal access token (PAT). The secret
	// store must have a secret named 'pat', containing the PAT value. A secret named
	// 'username' is optional, containing the username associated with the pat. By default no username is specified.
	Secret     string `json:"secret,omitempty"`
	SecretData map[string]string
}

// EnvironmentVariables represents the environment variables to be set for the recipe execution.
type EnvironmentVariables struct {
	// AdditionalProperties represents the non-sensitive environment variables to be set for the recipe execution.
	AdditionalProperties map[string]string `json:"additionalProperties,omitempty"`
}

type ProviderConfigProperties struct {
	// AdditionalProperties represents the non-sensitive environment variables to be set for the recipe execution.
	AdditionalProperties map[string]any `json:"additionalProperties,omitempty"`
}
