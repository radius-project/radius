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

import (
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
)

const TerraformConfigResourceType = "Radius.Core/terraformConfigs"

// TerraformConfig represents the Radius.Core/terraformConfigs resource.
type TerraformConfig struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties TerraformConfigResourceProperties `json:"properties"`
}

// ResourceTypeName returns the resource type of the TerraformConfig instance.
func (r *TerraformConfig) ResourceTypeName() string {
	return TerraformConfigResourceType
}

// TerraformConfigResourceProperties represents the properties of the Terraform config resource.
type TerraformConfigResourceProperties struct {
	// Authentication information used to access private Terraform module sources.
	Authentication AuthConfig `json:"authentication"`

	// Providers specifies the Terraform provider configurations.
	Providers map[string][]ProviderConfigProperties `json:"providers,omitempty"`

	// Env specifies the environment variables to be set during Terraform recipe execution.
	Env EnvironmentVariables `json:"env"`

	// EnvSecrets represents the secret-backed environment variables for recipe execution.
	EnvSecrets map[string]SecretReference `json:"envSecrets,omitempty"`

	// ReferencedBy is a list of environment IDs that reference this config.
	ReferencedBy []string `json:"referencedBy,omitempty"`
}
