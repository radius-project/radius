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

package terraform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestConfigureTerraformRegistry_ModuleRegistry(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Test module registry configuration
	const (
		registryHost  = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		token         = "test-auth-token-12345"
	)

	// Setup configuration with module registry
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
					"example-registry": {
						Host: registryHost,
						Authentication: datamodel.RegistryAuthConfig{
							Token: &datamodel.TokenConfig{
								Secret: secretStoreID,
							},
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"token": token,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")
	require.NotNil(t, regConfig, "Should return a RegistryConfig")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")
	require.Equal(t, configFilePath, regConfig.ConfigPath, "RegistryConfig should contain the correct config path")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content contains credentials block for module registry
	require.True(t, strings.Contains(configContent, "credentials"),
		"Config file should contain credentials block for module registry")
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`credentials "%s"`, registryHost)),
		"Config file should contain credentials block for the registry host")
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`token = "%s"`, token)),
		"Config file should contain the token in credentials block")

	// Verify NO provider installation block (that's for provider mirrors)
	require.False(t, strings.Contains(configContent, "provider_installation"),
		"Config file should NOT contain provider_installation block for module registries")

	// Verify environment variables are in the map (not set on process)
	require.Equal(t, configFilePath, regConfig.EnvVars[EnvTerraformCLIConfigFile],
		"TF_CLI_CONFIG_FILE should be in EnvVars map")

	// Module registries use credentials blocks, not TF_TOKEN_* env vars
	require.NotContains(t, regConfig.EnvVars, "TF_TOKEN_terraform_example_com",
		"Module registries should not use TF_TOKEN_* environment variables")

	// Verify the RegistryConfig tracks the environment variables
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile,
		"RegistryConfig should track TF_CLI_CONFIG_FILE")
	require.Len(t, regConfig.EnvVars, 1,
		"Should only have TF_CLI_CONFIG_FILE for module registries")

	// Test cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err, "Cleanup should not return an error")
	// Note: We no longer set/unset process environment variables
}

func TestConfigureTerraformRegistry_ProviderMirror_NoAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Setup configuration with provider mirror but no auth
	const mirrorURL = "terraform.example.com"
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
				},
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")
	require.NotNil(t, regConfig, "Should return a RegistryConfig")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content only contains the mirror section (no credentials)
	require.False(t, strings.Contains(configContent, "credentials"),
		"Config file should not contain credentials block when no auth is provided")

	// Check for provider installation block with mirror URL
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, `url     = "https://terraform.example.com"`),
		"Config file should contain the normalized mirror URL")

	// Verify only TF_CLI_CONFIG_FILE is tracked (no token env vars)
	require.Len(t, regConfig.EnvVars, 1, "Should only track one environment variable")
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile,
		"Should track TF_CLI_CONFIG_FILE")
}

func TestConfigureTerraformRegistry_NoRegistry(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Setup configuration with no registry
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")
	require.Nil(t, regConfig, "Should return nil when no registry is configured")

	// Verify the .terraformrc file was NOT created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	_, err = os.Stat(configFilePath)
	require.True(t, os.IsNotExist(err), "No .terraformrc file should be created when no registry is configured")
}

func TestConfigureTerraformRegistry_ProviderMirror_WithAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com:8443"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		token         = "test-token-with-port"
	)

	// Setup configuration with provider mirror and authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Token: &datamodel.TokenConfig{
							Secret: secretStoreID,
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"token": token,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the correct token env var name with port (colons replaced with underscores)
	require.Equal(t, token, regConfig.EnvVars["TF_TOKEN_terraform_example_com_8443"],
		"TF_TOKEN_* should include port with colons replaced by underscores")

	// Read the generated file
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the normalized URL includes the port
	require.True(t, strings.Contains(configContent, `url     = "https://terraform.example.com:8443"`),
		"Config file should contain the normalized mirror URL with port")

	// Should contain provider_installation block for provider mirror
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")

	// Should contain credentials block for provider mirror authentication
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`credentials "%s"`, mirrorURL)),
		"Config file should contain credentials block for provider mirror")
}

func TestConfigureTerraformRegistry_ModuleRegistry_MissingToken(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		registryHost  = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
	)

	// Setup configuration with token authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
					"example-registry": {
						Host: registryHost,
						Authentication: datamodel.RegistryAuthConfig{
							Token: &datamodel.TokenConfig{
								Secret: secretStoreID,
							},
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data with missing token
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				// token is missing
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.Error(t, err, "ConfigureTerraformRegistry should return an error when token is missing")
	require.Nil(t, regConfig, "Should return nil on error")
	require.Contains(t, err.Error(), "token not found")
}

func TestConfigureTerraformRegistry_ModuleRegistry_AdditionalHosts(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		registryHost  = "my-registry.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecrets"
		token         = "test-token-12345"
	)

	// Setup configuration with additional hosts
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
					"example-registry": {
						Host: registryHost,
						Authentication: datamodel.RegistryAuthConfig{
							Token: &datamodel.TokenConfig{
								Secret: secretStoreID,
							},
							AdditionalHosts: []string{"original-registry.example.com", "backup-registry.example.com"},
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"token": token,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Read the generated file
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Check that credentials blocks are created for all hosts
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`credentials "%s"`, registryHost)),
		"Config file should contain credentials block for primary host")
	require.True(t, strings.Contains(configContent, `credentials "original-registry.example.com"`),
		"Config file should contain credentials block for first additional host")
	require.True(t, strings.Contains(configContent, `credentials "backup-registry.example.com"`),
		"Config file should contain credentials block for second additional host")

	// Verify all credentials blocks have the same token
	tokenCount := strings.Count(configContent, fmt.Sprintf(`token = "%s"`, token))
	require.Equal(t, 3, tokenCount, "Should have 3 credentials blocks with the same token")

	// Cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)
}

func TestConfigureTerraformRegistry_ProviderMirror_InvalidURL(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Setup configuration with invalid URL
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: "://invalid-url",
				},
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.Error(t, err, "ConfigureTerraformRegistry should return an error for invalid URL")
	require.Nil(t, regConfig, "Should return nil on error")
	require.Contains(t, err.Error(), "invalid terraform registry mirror URL")
}

func TestConfigureTerraformRegistry_BothProviderMirrorAndModuleRegistry(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		providerMirrorURL      = "providers.example.com"
		providerSecretStoreID  = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/providerSecrets"
		providerToken          = "provider-token-12345"
		moduleRegistryHost     = "modules.example.com"
		moduleSecretStoreID    = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/moduleSecrets"
		moduleToken            = "module-token-67890"
	)

	// Setup configuration with both provider mirror and module registry
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: providerMirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Token: &datamodel.TokenConfig{
							Secret: providerSecretStoreID,
						},
					},
				},
				ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
					"example-modules": {
						Host: moduleRegistryHost,
						Authentication: datamodel.RegistryAuthConfig{
							Token: &datamodel.TokenConfig{
								Secret: moduleSecretStoreID,
							},
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		providerSecretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"token": providerToken,
			},
		},
		moduleSecretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"token": moduleToken,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")
	require.NotNil(t, regConfig, "Should return a RegistryConfig")

	// Read the generated file
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify provider mirror configuration
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, `url     = "https://providers.example.com"`),
		"Config file should contain provider mirror URL")

	// Verify provider mirror credentials
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`credentials "%s"`, providerMirrorURL)),
		"Config file should contain credentials block for provider mirror")
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`token = "%s"`, providerToken)),
		"Config file should contain provider token")

	// Verify module registry credentials
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`credentials "%s"`, moduleRegistryHost)),
		"Config file should contain credentials block for module registry")
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`token = "%s"`, moduleToken)),
		"Config file should contain module token")

	// Verify environment variables
	require.Equal(t, providerToken, regConfig.EnvVars["TF_TOKEN_providers_example_com"],
		"Should have provider token in environment variables")
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile,
		"Should have TF_CLI_CONFIG_FILE")

	// Module registries should not have TF_TOKEN_* env vars (they use credentials blocks only)
	require.NotContains(t, regConfig.EnvVars, "TF_TOKEN_modules_example_com",
		"Module registries should not use TF_TOKEN_* environment variables")

	// Cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)
}

func TestCleanupTerraformRegistryConfig_NilConfig(t *testing.T) {
	// Test that cleanup handles nil config gracefully
	ctx := context.Background()
	err := CleanupTerraformRegistryConfig(ctx, nil)
	require.NoError(t, err, "Cleanup should handle nil config without error")
}

func TestCleanupTerraformRegistryConfig_FileRemoval(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test.terraformrc")

	// Create a test file
	err := os.WriteFile(configPath, []byte("test"), 0600)
	require.NoError(t, err)

	// Create config
	regConfig := &RegistryConfig{
		ConfigPath: configPath,
		EnvVars:    map[string]string{"TEST_VAR": "test-value"},
	}

	// Call cleanup
	ctx := context.Background()
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)

	// Verify file is removed
	_, err = os.Stat(configPath)
	require.True(t, os.IsNotExist(err), "Config file should be removed")

	// Note: We no longer unset environment variables since they are now passed
	// to the Terraform process rather than set on the current process
}

func TestConfigureTerraformRegistry_ProviderMirror_WithCACert(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL         = "https://my-private-registry.company.com"
		tokenSecretID     = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/tokens"
		caCertSecretID    = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/certs"
		token             = "private-registry-token"
		caCertContent     = `-----BEGIN CERTIFICATE-----
MIIDQTCCAimgAwIBAgITBmyfz5m/jAo54vB4ikPmljZbyjANBgkqhkiG9w0BAQsF
ADA5MQswCQYDVQQGEwJVUzEPMA0GA1UEChMGQW1hem9uMRkwFwYDVQQDExBBbWF6
b24gUm9vdCBDQSAxMA0GCSqGSIb3DQEBCwUAA4IBAQCTLMF4dYaD+3yL4FyYLG2o
-----END CERTIFICATE-----`
	)

	// Setup configuration with provider mirror, authentication, and CA certificate
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Token: &datamodel.TokenConfig{
							Secret: tokenSecretID,
						},
					},
					TLS: &datamodel.TLSConfig{
						CACertificate: &datamodel.SecretReference{
							Source: caCertSecretID,
							Key:    "ca.crt",
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data with both token and CA certificate
	secrets := map[string]recipes.SecretData{
		tokenSecretID: {
			Type: "opaque",
			Data: map[string]string{
				"token": token,
			},
		},
		caCertSecretID: {
			Type: "opaque",
			Data: map[string]string{
				"ca.crt": caCertContent,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")
	require.NotNil(t, regConfig, "Should return a RegistryConfig")

	// Verify CA certificate file was created
	require.Len(t, regConfig.TempFiles, 1, "Should have one temporary file for CA certificate")
	caCertPath := regConfig.TempFiles[0]
	require.True(t, strings.HasSuffix(caCertPath, "terraform-registry-ca.pem"), "CA cert file should have correct name")
	require.FileExists(t, caCertPath, "CA certificate file should exist")

	// Verify CA certificate content
	writtenCert, err := os.ReadFile(caCertPath)
	require.NoError(t, err, "Should be able to read CA certificate file")
	require.Equal(t, caCertContent, string(writtenCert), "CA certificate content should match")

	// Verify CA certificate environment variables
	require.Equal(t, caCertPath, regConfig.EnvVars["SSL_CERT_FILE"],
		"SSL_CERT_FILE should point to CA certificate")
	require.Equal(t, caCertPath, regConfig.EnvVars["CURL_CA_BUNDLE"],
		"CURL_CA_BUNDLE should point to CA certificate")

	// Verify .terraformrc contains provider installation block
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read config file")
	configContent := string(content)
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config should contain provider_installation block")

	// Cleanup should remove CA certificate file
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err, "Cleanup should succeed")
	require.NoFileExists(t, caCertPath, "CA certificate file should be removed after cleanup")
}

func TestConfigureTerraformRegistry_ProviderMirror_TLSSkipVerify(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "https://self-signed-registry.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/tokens"
		token         = "insecure-registry-token"
	)

	// Setup configuration with TLS skip verify
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Token: &datamodel.TokenConfig{
							Secret: secretStoreID,
						},
					},
					TLS: &datamodel.TLSConfig{
						SkipVerify: true,
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"token": token,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify TLS skip verify environment variable is set
	require.Equal(t, "1", regConfig.EnvVars["TF_INSECURE_SKIP_TLS_VERIFY"],
		"TF_INSECURE_SKIP_TLS_VERIFY should be set to 1")

	// Verify no CA certificate file is created
	require.Len(t, regConfig.TempFiles, 0, "Should not create any temporary files for skip verify")

	// Cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)
}

func TestConfigureTerraformRegistry_ProviderMirror_CACert_MissingSecret(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL      = "https://private-registry.example.com"
		caCertSecretID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/missing"
	)

	// Setup configuration with CA certificate but missing secret
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
					TLS: &datamodel.TLSConfig{
						CACertificate: &datamodel.SecretReference{
							Source: caCertSecretID,
							Key:    "ca.crt",
						},
					},
				},
			},
		},
	}

	// No secrets provided
	secrets := map[string]recipes.SecretData{}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.Error(t, err, "Should return error when CA certificate secret is missing")
	require.Nil(t, regConfig, "Should return nil on error")
	require.Contains(t, err.Error(), "secret store", "Error should mention missing secret store")
}

func TestConfigureTerraformRegistry_ProviderMirror_CACert_MissingKey(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL      = "https://private-registry.example.com"
		caCertSecretID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/certs"
	)

	// Setup configuration with CA certificate
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
					TLS: &datamodel.TLSConfig{
						CACertificate: &datamodel.SecretReference{
							Source: caCertSecretID,
							Key:    "ca.crt",
						},
					},
				},
			},
		},
	}

	// Secret exists but doesn't have the required key
	secrets := map[string]recipes.SecretData{
		caCertSecretID: {
			Type: "opaque",
			Data: map[string]string{
				"other-key": "some-other-data",
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.Error(t, err, "Should return error when CA certificate key is missing")
	require.Nil(t, regConfig, "Should return nil on error")
	require.Contains(t, err.Error(), "CA certificate not found", "Error should mention missing CA certificate")
}

func TestConfigureTerraformRegistry_ProviderMirror_TLSSkipVerifyAndCACert(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL         = "https://registry-with-custom-ca.example.com"
		caCertSecretID    = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/certs"
		caCertContent     = `-----BEGIN CERTIFICATE-----
MIIDCustomCertForTesting
-----END CERTIFICATE-----`
	)

	// Setup configuration with both skip verify AND CA certificate (skip verify should take precedence)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Mirror: mirrorURL,
					TLS: &datamodel.TLSConfig{
						SkipVerify: true,
						CACertificate: &datamodel.SecretReference{
							Source: caCertSecretID,
							Key:    "ca.crt",
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		caCertSecretID: {
			Type: "opaque",
			Data: map[string]string{
				"ca.crt": caCertContent,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify TLS skip verify is set
	require.Equal(t, "1", regConfig.EnvVars["TF_INSECURE_SKIP_TLS_VERIFY"],
		"TF_INSECURE_SKIP_TLS_VERIFY should be set")

	// CA certificate should still be processed (both can coexist)
	require.Len(t, regConfig.TempFiles, 1, "Should create CA certificate file")
	require.Contains(t, regConfig.EnvVars, "SSL_CERT_FILE", "Should set SSL_CERT_FILE")
	require.Contains(t, regConfig.EnvVars, "CURL_CA_BUNDLE", "Should set CURL_CA_BUNDLE")

	// Cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)
}
