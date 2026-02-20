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

import v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"

const TerraformSettingsResourceType_v20250801preview = "Radius.Core/terraformSettings"

// TerraformSettings_v20250801preview represents the Radius.Core/terraformSettings resource.
type TerraformSettings_v20250801preview struct {
	v1.BaseResource

	// Properties of the Terraform settings resource.
	Properties TerraformSettingsProperties_v20250801preview `json:"properties"`
}

// ResourceTypeName returns the resource type for Terraform settings.
func (t *TerraformSettings_v20250801preview) ResourceTypeName() string {
	return TerraformSettingsResourceType_v20250801preview
}

// TerraformSettingsProperties_v20250801preview describes the Terraform settings payload.
type TerraformSettingsProperties_v20250801preview struct {
	// TerraformRC mirrors the terraformrc file shape (provider mirrors, credentials).
	TerraformRC *TerraformCliConfiguration `json:"terraformrc,omitempty"`

	// Backend configuration matching the Terraform backend block.
	Backend *TerraformBackendConfiguration `json:"backend,omitempty"`

	// Env contains environment variables passed to Terraform executions.
	Env map[string]string `json:"env,omitempty"`

	// Logging controls Terraform logging behaviour (TF_LOG/TF_LOG_PATH).
	Logging *TerraformLoggingConfiguration `json:"logging,omitempty"`
}

// TerraformCliConfiguration mirrors the terraformrc provider installation + credentials sections.
type TerraformCliConfiguration struct {
	ProviderInstallation *TerraformProviderInstallationConfiguration `json:"providerInstallation,omitempty"`
	Credentials          map[string]*TerraformCredentialConfiguration `json:"credentials,omitempty"`
}

// TerraformProviderInstallationConfiguration describes network mirror and direct rules.
type TerraformProviderInstallationConfiguration struct {
	NetworkMirror *TerraformNetworkMirrorConfiguration `json:"networkMirror,omitempty"`
	Direct        *TerraformDirectConfiguration        `json:"direct,omitempty"`
}

// TerraformNetworkMirrorConfiguration describes a network mirror entry.
type TerraformNetworkMirrorConfiguration struct {
	URL            string                 `json:"url"`
	Include        []string               `json:"include,omitempty"`
	Exclude        []string               `json:"exclude,omitempty"`
}

// TerraformDirectInstallationConfiguration controls direct installation rules.
type TerraformDirectConfiguration struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

// TerraformCredentialConfiguration describes credentials keyed by hostname.
type TerraformCredentialConfiguration struct {
	Token *SecretRef `json:"token,omitempty"`
}

// SecretRef points to a secret in Radius.Security/secrets.
// This is separate from SecretReference in recipe_types.go which uses different field names.
type SecretRef struct {
	SecretID string `json:"secretId"`
	Key      string `json:"key"`
}

// TerraformBackendConfiguration mirrors the Terraform backend block (type + config).
type TerraformBackendConfiguration struct {
	Type   string            `json:"type"`
	Config map[string]string `json:"config,omitempty"`
}

// TerraformLoggingConfiguration captures TF_LOG/TF_LOG_PATH settings.
type TerraformLoggingConfiguration struct {
	Level TerraformLogLevel `json:"level,omitempty"`
	Path  string            `json:"path,omitempty"`
}

// TerraformLogLevel enumerates supported TF_LOG values.
type TerraformLogLevel string

const (
	TerraformLogLevelTrace TerraformLogLevel = "TRACE"
	TerraformLogLevelDebug TerraformLogLevel = "DEBUG"
	TerraformLogLevelInfo  TerraformLogLevel = "INFO"
	TerraformLogLevelWarn  TerraformLogLevel = "WARN"
	TerraformLogLevelError TerraformLogLevel = "ERROR"
	TerraformLogLevelFatal TerraformLogLevel = "FATAL"
)
