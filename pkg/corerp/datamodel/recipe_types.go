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

	// BicepConfigProperties represents configuration for Bicep Recipes. Controls how Bicep plans and applies templates as part of Recipe deployment.
	Bicep BicepConfigProperties `json:"bicep,omitempty"`

	// Env specifies the environment variables to be set during the Terraform Recipe execution.
	Env EnvironmentVariables `json:"env,omitempty"`

	// EnvSecrets represents the environment secrets for the recipe.
	// The keys of the map are the names of the secrets, and the values are the references to the secrets.
	EnvSecrets map[string]SecretReference `json:"envSecrets,omitempty"`
}

// TerraformConfigProperties - Configuration for Terraform Recipes. Controls how Terraform plans and applies templates as
// part of Recipe deployment.
type TerraformConfigProperties struct {
	// Authentication information used to access private Terraform module sources. Supported module sources: Git.
	Authentication AuthConfig `json:"authentication,omitempty"`

	// Providers specifies the Terraform provider configurations. Controls how Terraform interacts with cloud providers, SaaS providers, and other APIs: https://developer.hashicorp.com/terraform/language/providers/configuration.// Providers specifies the Terraform provider configurations.
	Providers map[string][]ProviderConfigProperties `json:"providers,omitempty"`

	// Registry specifies the Terraform registry configuration.
	Registry *TerraformRegistryConfig `json:"registry,omitempty"`

	// Version specifies the Terraform binary version and the URL to download it from.
	Version *TerraformVersionConfig `json:"version,omitempty"`
}

// BicepConfigProperties - Configuration for Bicep Recipes. Controls how Bicep plans and applies templates as part of Recipe
// deployment.
type BicepConfigProperties struct {
	// Authentication holds the information used to access private bicep registries, which is a map of registry hostname to secret config
	// that contains credential information.
	Authentication map[string]RegistrySecretConfig
}

// RegistrySecretConfig - Registry Secret Configuration used to authenticate to private bicep registries.
type RegistrySecretConfig struct {
	// Secret is the ID of an Applications.Core/SecretStore resource containing credential information used to authenticate private
	// container registry. The keys in the secretstore depends on the type.
	Secret string
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

	// SSH key configuration used to authenticate to Git platforms.
	SSH map[string]SSHConfig `json:"ssh,omitempty"`
}

// SecretConfig - Personal Access Token (PAT) configuration used to authenticate to Git platforms.
type SecretConfig struct {
	// The ID of an Applications.Core/SecretStore resource containing the Git platform personal access token (PAT). The secret
	// store must have a secret named 'pat', containing the PAT value. A secret named
	// 'username' is optional, containing the username associated with the pat. By default no username is specified.
	Secret string `json:"secret,omitempty"`
}

// SSHConfig - SSH key configuration used to authenticate to Git platforms.
type SSHConfig struct {
	// The ID of an Applications.Core/SecretStore resource containing the SSH private key and optional passphrase.
	// The secret store must have a secret named 'private-key' containing the SSH private key.
	// A secret named 'passphrase' is optional, containing the passphrase for the private key.
	// A secret named 'known-hosts' is optional, containing the known hosts entries.
	Secret string `json:"secret,omitempty"`

	// StrictHostKeyChecking specifies whether to perform strict host key checking.
	// When false, accepts any host key (insecure but sometimes necessary for private Git servers).
	// Defaults to true.
	StrictHostKeyChecking bool `json:"strictHostKeyChecking,omitempty"`
}


// ClientCertConfig - Client certificate (mTLS) configuration for authentication.
type ClientCertConfig struct {
	// The ID of an Applications.Core/SecretStore resource containing the client certificate and key.
	// The secret store must have secrets named 'cert' and 'key' containing the PEM-encoded certificate and private key.
	// A secret named 'passphrase' is optional, containing the passphrase for the private key.
	Secret string `json:"secret,omitempty"`
}

// EnvironmentVariables represents the environment variables to be set for the recipe execution.
type EnvironmentVariables struct {
	// AdditionalProperties represents the non-sensitive environment variables to be set for the recipe execution.
	AdditionalProperties map[string]string `json:"additionalProperties,omitempty"`
}

type ProviderConfigProperties struct {
	// AdditionalProperties represents the non-sensitive environment variables to be set for the recipe execution.
	AdditionalProperties map[string]any `json:"additionalProperties,omitempty"`

	// Secrets represents the secrets to be set for recipe execution in the current Provider configuration.
	Secrets map[string]SecretReference `json:"secrets,omitempty"`
}

// SecretReference represents a reference to a secret.
type SecretReference struct {
	// Source represents the Secret Store ID of the secret.
	Source string `json:"source"`

	// Key represents the key of the secret.
	Key string `json:"key"`
}

// TerraformRegistryConfig - Configuration for Terraform Registry.
type TerraformRegistryConfig struct {
	// Mirror is the URL to use instead of the default Terraform registry. Example: 'https://terraform.example.com'.
	Mirror string `json:"mirror,omitempty"`

	// ProviderMappings is used to translate between official and custom provider identifiers.
	ProviderMappings map[string]string `json:"providerMappings,omitempty"`

	// Authentication configuration for accessing private Terraform registry mirrors.
	Authentication RegistryAuthConfig `json:"authentication,omitempty"`
}

// TokenConfig - Token authentication configuration.
type TokenConfig struct {
	// The ID of an Applications.Core/SecretStore resource containing the authentication token.
	// The secret store must have a secret named 'token' containing the token value.
	Secret string `json:"secret,omitempty"`
}

// RegistryAuthConfig - Authentication configuration for accessing private Terraform registry mirrors.
type RegistryAuthConfig struct {
	// Token is the token authentication configuration for registry authentication.
	Token *TokenConfig `json:"token,omitempty"`

	// AdditionalHosts is a list of additional hosts that should use the same authentication credentials.
	// This is useful when a registry mirror redirects to other hosts (e.g., GitLab Pages mirrors redirecting to gitlab.com).
	AdditionalHosts []string `json:"additionalHosts,omitempty"`
}

// TerraformVersionConfig - Configuration for Terraform binary.
type TerraformVersionConfig struct {
	// Version is the version of the Terraform binary to use. Example: '1.0.0'.
	// If omitted, the system may default to the latest stable version.
	Version string `json:"version,omitempty"`

	// ReleasesArchiveURL is an optional direct URL to a Terraform binary archive (.zip file).
	// If set, Terraform will be downloaded directly from this URL instead of using the releases API.
	// This takes precedence over ReleasesAPIBaseURL.
	// The URL must point to a valid Terraform release archive.
	// Example: 'https://my-mirror.example.com/terraform/1.7.0/terraform_1.7.0_linux_amd64.zip'
	ReleasesArchiveURL string `json:"releasesArchiveUrl,omitempty"`

	// ReleasesAPIBaseURL is an optional base URL for a custom Terraform releases API.
	// If set, Terraform will be downloaded from this base URL instead of the default HashiCorp releases site.
	// The directory structure of the custom URL must match the HashiCorp releases site (including the index.json files).
	// Example: 'https://my-terraform-mirror.example.com'
	ReleasesAPIBaseURL string `json:"releasesApiBaseUrl,omitempty"`

	// TLS contains TLS configuration for connecting to the releases API.
	TLS *TerraformTLSConfig `json:"tls,omitempty"`

	// Authentication configuration for accessing the Terraform binary releases API.
	Authentication *RegistryAuthConfig `json:"authentication,omitempty"`
}

// TerraformTLSConfig - TLS configuration options for Terraform binary downloads.
type TerraformTLSConfig struct {
	// CACertificate is a reference to a secret containing a custom CA certificate bundle to use for TLS verification.
	// The secret must contain a key named 'ca-cert' with the PEM-encoded certificate bundle.
	CACertificate *SecretReference `json:"caCertificate,omitempty"`

	// ClientCertificate is the client certificate configuration for mutual TLS (mTLS) authentication.
	ClientCertificate *ClientCertConfig `json:"clientCertificate,omitempty"`

	// SkipVerify allows insecure connections (skip TLS verification).
	// This is strongly discouraged in production environments.
	// WARNING: This makes the connection vulnerable to man-in-the-middle attacks.
	SkipVerify bool `json:"skipVerify,omitempty"`
}
