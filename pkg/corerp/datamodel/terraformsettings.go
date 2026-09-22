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
	"fmt"
	"regexp"
	"strings"

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
	// Backend selects remote state storage. Nil preserves the Kubernetes backend.
	Backend *TerraformBackend `json:"backend,omitempty"`

	// Terraformrc contains Terraform CLI configuration file (.terraformrc) settings.
	Terraformrc TerraformrcConfig `json:"terraformrc"`

	// Env specifies the environment variables to be set during Terraform recipe execution.
	Env map[string]string `json:"env,omitempty"`

	// ReferencedBy is a list of environment IDs that reference this config.
	ReferencedBy []string `json:"referencedBy,omitempty"`
}

// TerraformBackend is a cloud state location, without authentication material.
type TerraformBackend struct {
	Type               string  `json:"type"`
	Bucket             string  `json:"bucket,omitempty"`
	Region             string  `json:"region,omitempty"`
	StorageAccountName string  `json:"storageAccountName,omitempty"`
	ContainerName      string  `json:"containerName,omitempty"`
	KeyPrefix          *string `json:"keyPrefix,omitempty"`
}

// EffectiveKeyPrefix returns the default prefix when no prefix was supplied.
func (b *TerraformBackend) EffectiveKeyPrefix() string {
	if b.KeyPrefix == nil {
		return "radius"
	}
	return *b.KeyPrefix
}

var backendKeyPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*$`)

// Validate rejects unknown variants, cross-cloud fields and ambiguous state prefixes.
func (b *TerraformBackend) Validate() error {
	if b == nil {
		return nil
	}
	// Both clouds use S3's stricter limit: 1024 bytes minus "/" + 40 hex digits + ".tfstate.tflock".
	if len(b.EffectiveKeyPrefix()) > 968 || !backendKeyPrefixPattern.MatchString(b.EffectiveKeyPrefix()) {
		return fmt.Errorf("backend.keyPrefix must be 1-968 characters with nonempty slash-separated segments of letters, digits, underscores or hyphens")
	}
	switch b.Type {
	case "s3":
		if strings.TrimSpace(b.Bucket) == "" || strings.TrimSpace(b.Region) == "" ||
			b.Bucket != strings.TrimSpace(b.Bucket) || b.Region != strings.TrimSpace(b.Region) {
			return fmt.Errorf("backend.bucket and backend.region are required for s3 and must not have surrounding whitespace")
		}
		if b.StorageAccountName != "" || b.ContainerName != "" {
			return fmt.Errorf("s3 backend cannot specify storageAccountName or containerName")
		}
	case "azurerm":
		if strings.TrimSpace(b.StorageAccountName) == "" || strings.TrimSpace(b.ContainerName) == "" ||
			b.StorageAccountName != strings.TrimSpace(b.StorageAccountName) || b.ContainerName != strings.TrimSpace(b.ContainerName) {
			return fmt.Errorf("backend.storageAccountName and backend.containerName are required for azurerm and must not have surrounding whitespace")
		}
		if b.Bucket != "" || b.Region != "" {
			return fmt.Errorf("azurerm backend cannot specify bucket or region")
		}
	default:
		return fmt.Errorf("backend.type must be s3 or azurerm")
	}
	return nil
}

// SameLocation compares effective state locations, treating the default prefix identically.
func (b *TerraformBackend) SameLocation(other *TerraformBackend) bool {
	if b == nil || other == nil {
		return b == other
	}
	return b.Type == other.Type && b.Bucket == other.Bucket && b.Region == other.Region &&
		b.StorageAccountName == other.StorageAccountName && b.ContainerName == other.ContainerName &&
		b.EffectiveKeyPrefix() == other.EffectiveKeyPrefix()
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
