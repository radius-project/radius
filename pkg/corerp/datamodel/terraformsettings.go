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

const TerraformSettingsResourceType = "Radius.Core/terraformSettings"

// TerraformSettings represents the Radius.Core/terraformSettings resource.
type TerraformSettings struct {
	v1.BaseResource

	// Properties is the properties of the resource.
	Properties TerraformSettingsResourceProperties `json:"properties"`
}

// ResourceTypeName returns the resource type of the TerraformSettings instance.
func (r *TerraformSettings) ResourceTypeName() string {
	return TerraformSettingsResourceType
}

// TerraformSettingsResourceProperties represents the properties of the Terraform config resource.
type TerraformSettingsResourceProperties struct {
	// Terraformrc contains Terraform CLI configuration file (.terraformrc) settings.
	Terraformrc TerraformrcConfig `json:"terraformrc"`

	// Env specifies the environment variables to be set during Terraform recipe execution.
	Env map[string]string `json:"env,omitempty"`

	// ReferencedBy is a list of environment IDs that reference this config.
	ReferencedBy []string `json:"referencedBy,omitempty"`
}

// TerraformrcConfig represents .terraformrc settings.
type TerraformrcConfig struct {
	// ProviderInstallation configures provider mirror and direct installation.
	ProviderInstallation *TerraformProviderInstallation `json:"providerInstallation,omitempty"`

	// Credentials maps registry/module hostnames to credential configuration.
	Credentials map[string]TerraformCredentialConfig `json:"credentials,omitempty"`
}

// TerraformProviderInstallation configures how Terraform resolves providers.
type TerraformProviderInstallation struct {
	// NetworkMirror configures a network mirror for provider downloads.
	NetworkMirror *TerraformProviderMirror `json:"networkMirror,omitempty"`

	// Direct configures direct provider installation.
	Direct *TerraformProviderDirect `json:"direct,omitempty"`
}

// TerraformProviderMirror configures a network mirror for Terraform providers.
type TerraformProviderMirror struct {
	URL     string   `json:"url,omitempty"`
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// TerraformProviderDirect configures direct provider installation.
type TerraformProviderDirect struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// TerraformCredentialConfig holds credential information for a Terraform registry host.
type TerraformCredentialConfig struct {
	// Secret is the ID of a secret resource containing the authentication token.
	// Supported types: Radius.Security/secrets or Applications.Core/secretStores.
	Secret string `json:"secret,omitempty"`
}
