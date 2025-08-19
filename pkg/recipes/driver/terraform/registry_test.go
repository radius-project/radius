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
	"encoding/base64"
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
	require.Contains(t, regConfig.EnvVars, "GIT_CONFIG_GLOBAL",
		"RegistryConfig should track GIT_CONFIG_GLOBAL for Git authentication")
	require.Contains(t, regConfig.EnvVars, "HOME",
		"RegistryConfig should track HOME for Git authentication")
	require.Len(t, regConfig.EnvVars, 4,
		"Should have TF_CLI_CONFIG_FILE plus Git authentication environment variables")

	// Test cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err, "Cleanup should not return an error")
	// Note: We no longer set/unset process environment variables
}

func TestConfigureTerraformRegistry_ProviderMirror_NoAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Setup configuration with provider mirror using network protocol (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "https://mirror.example.com/providers/",
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

	// Check for provider installation block with network mirror
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, "network_mirror {"),
		"Config file should contain network_mirror block")
	require.Contains(t, configContent, `url = "https://mirror.example.com/providers"`,
		"Config file should contain the mirror URL without trailing slash")

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
	_, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	_, err = os.Stat(configFilePath)
	require.True(t, os.IsNotExist(err), "No .terraformrc file should be created when no registry is configured")
}

func TestConfigureTerraformRegistry_ProviderMirror_WithAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		token         = "test-token-with-port"
	)

	// Setup configuration with provider mirror and authentication (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "https://mirror.example.com/providers/",
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

	// Read the generated file
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Should contain provider_installation block with network mirror
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, "network_mirror {"),
		"Config file should contain network_mirror block")
	require.Contains(t, configContent, `url = "https://mirror.example.com/providers"`,
		"Config file should contain the mirror URL without trailing slash")

	// Should only track TF_CLI_CONFIG_FILE for network mirror
	require.Len(t, regConfig.EnvVars, 1, "Should only track TF_CLI_CONFIG_FILE for network mirror")
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile)
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

	// Setup configuration with empty URL (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "", // Empty URL should cause error
				},
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.Error(t, err, "ConfigureTerraformRegistry should return an error for empty URL")
	require.Nil(t, regConfig, "Should return nil on error")
	require.Contains(t, err.Error(), "provider mirror url is required")
}

func TestConfigureTerraformRegistry_BothProviderMirrorAndModuleRegistry(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		providerSecretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/providerSecrets"
		providerToken         = "provider-token-12345"
		moduleRegistryHost    = "modules.example.com"
		moduleSecretStoreID   = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/moduleSecrets"
		moduleToken           = "module-token-67890"
	)

	// Setup configuration with both provider mirror and module registry (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "https://mirror.example.com/providers/",
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
	require.True(t, strings.Contains(configContent, "network_mirror {"),
		"Config file should contain network_mirror block")
	require.Contains(t, configContent, `url = "https://mirror.example.com/providers"`,
		"Config file should contain provider mirror URL without trailing slash")

	// Verify module registry credentials
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`credentials "%s"`, moduleRegistryHost)),
		"Config file should contain credentials block for module registry")
	require.True(t, strings.Contains(configContent, fmt.Sprintf(`token = "%s"`, moduleToken)),
		"Config file should contain module token")

	// Verify environment variables include TF_CLI_CONFIG_FILE and Git authentication variables
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile,
		"Should have TF_CLI_CONFIG_FILE")
	require.Contains(t, regConfig.EnvVars, "GIT_CONFIG_GLOBAL",
		"Should have GIT_CONFIG_GLOBAL for Git authentication")
	require.Contains(t, regConfig.EnvVars, "HOME",
		"Should have HOME for Git authentication")
	require.Len(t, regConfig.EnvVars, 4, "Should have TF_CLI_CONFIG_FILE plus Git authentication variables")

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
		tokenSecretID  = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/tokens"
		caCertSecretID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/certs"
		token          = "private-registry-token"
		caCertContent  = `-----BEGIN CERTIFICATE-----
MIIDQTCCAimgAwIBAgITBmyfz5m/jAo54vB4ikPmljZbyjANBgkqhkiG9w0BAQsF
ADA5MQswCQYDVQQGEwJVUzEPMA0GA1UEChMGQW1hem9uMRkwFwYDVQQDExBBbWF6
b24gUm9vdCBDQSAxMA0GCSqGSIb3DQEBCwUAA4IBAQCTLMF4dYaD+3yL4FyYLG2o
-----END CERTIFICATE-----`
	)

	// Setup configuration with provider mirror, authentication, and CA certificate (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "https://mirror.example.com/providers/",
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

	// Verify CA certificate configuration is ignored for provider mirrors (no auth/TLS support)
	require.Len(t, regConfig.TempFiles, 0, "Provider mirrors do not support TLS - no CA certificate files should be created")

	// Verify no TLS environment variables are set for provider mirrors
	_, hasSSL := regConfig.EnvVars["SSL_CERT_FILE"]
	require.False(t, hasSSL, "SSL_CERT_FILE should not be set for provider mirrors")
	_, hasCURL := regConfig.EnvVars["CURL_CA_BUNDLE"]
	require.False(t, hasCURL, "CURL_CA_BUNDLE should not be set for provider mirrors")

	// Verify .terraformrc contains provider installation block without authentication
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read config file")
	configContent := string(content)
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config should contain provider_installation block")
	require.True(t, strings.Contains(configContent, "network_mirror {"),
		"Config should contain network_mirror block")

	// Cleanup (no files to clean up for provider mirrors)
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err, "Cleanup should succeed")
}


func TestConfigureTerraformRegistry_ProviderMirror_CACert_MissingSecret(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		caCertSecretID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/missing"
	)

	// Setup configuration with CA certificate but missing secret (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "https://mirror.example.com/providers/",
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
	require.NoError(t, err, "Should succeed - TLS configuration is ignored for provider mirrors")
	require.NotNil(t, regConfig, "Should return valid config")

	// Verify no TLS environment variables or files are created
	require.Len(t, regConfig.TempFiles, 0, "No TLS files should be created for provider mirrors")
}

func TestConfigureTerraformRegistry_ProviderMirror_CACert_MissingKey(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		caCertSecretID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/certs"
	)

	// Setup configuration with CA certificate (type field is deprecated)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					URL: "https://mirror.example.com/providers/",
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
	require.NoError(t, err, "Should succeed - TLS configuration is ignored for provider mirrors")
	require.NotNil(t, regConfig, "Should return valid config")

	// Verify no TLS environment variables or files are created
	require.Len(t, regConfig.TempFiles, 0, "No TLS files should be created for provider mirrors")
}


func TestConfigureTerraformRegistry_GitLabAirGappedEnvironment(t *testing.T) {
	// Test configuration based on GitLab air-gapped environment with registry.terraform.io redirection
	tests := []struct {
		name                     string
		config                   recipes.Configuration
		secrets                  map[string]recipes.SecretData
		expectedHostBlocks       []string
		expectedCredentials      []string
		expectedEnvVars          map[string]string
		shouldHaveProviderMirror bool
	}{
		{
			name: "GitLab module registry with provider mirror - full air-gapped setup",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
							URL: "https://providermirror.example.com/providers/", // will be set to network URL
							Authentication: datamodel.RegistryAuthConfig{
								Token: &datamodel.TokenConfig{
									Secret: "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitlab-token",
								},
							},
							TLS: &datamodel.TLSConfig{},
						},
						ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
							"gitlab": {
								Host: "providermirror.example.com",
								Authentication: datamodel.RegistryAuthConfig{
									Token: &datamodel.TokenConfig{
										Secret: "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitlab-token",
									},
								},
							},
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitlab-token": {
					Data: map[string]string{
						// Example token for tests; not a real secret and intentionally avoids real PAT patterns
						"token": "example-test-token",
					},
				},
			},
			expectedHostBlocks: []string{
				`host "gitlab" {
  services = {
    "modules.v1" = "https://providermirror.example.com"
  }
}`,
			},
			expectedCredentials: []string{
				`credentials "providermirror.example.com" {
  token = "example-test-token"
}`,
			},
			expectedEnvVars: map[string]string{
				// Provider mirrors do not support TLS configuration
			},
			shouldHaveProviderMirror: true,
		},
		{
			name: "GitLab terraform-providers registry name - should redirect registry.terraform.io",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
							"terraform-providers": {
								Host: "providermirror.example.com",
								Authentication: datamodel.RegistryAuthConfig{
									Token: &datamodel.TokenConfig{
										Secret: "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitlab-token",
									},
								},
							},
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitlab-token": {
					Data: map[string]string{
						// Example token for tests; not a real secret and intentionally avoids real PAT patterns
						"token": "example-test-token",
					},
				},
			},
			expectedHostBlocks: []string{
				`host "terraform-providers" {
  services = {
    "modules.v1" = "https://providermirror.example.com"
  }
}`,
			},
			expectedCredentials: []string{
				`credentials "providermirror.example.com" {
  token = "example-test-token"
}`,
			},
			expectedEnvVars:          map[string]string{},
			shouldHaveProviderMirror: false,
		},
		{
			name: "Custom registry name - should NOT redirect registry.terraform.io",
			config: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Terraform: datamodel.TerraformConfigProperties{
						ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
							"company-internal": {
								Host: "internal.company.com",
								Authentication: datamodel.RegistryAuthConfig{
									Token: &datamodel.TokenConfig{
										Secret: "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/internal-token",
									},
								},
							},
						},
					},
				},
			},
			secrets: map[string]recipes.SecretData{
				"/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/internal-token": {
					Data: map[string]string{
						"token": "internal-token-value",
					},
				},
			},
			expectedHostBlocks: []string{
				`host "company-internal" {
  services = {
    "modules.v1" = "https://internal.company.com"
  }
}`,
			},
			expectedCredentials: []string{
				`credentials "internal.company.com" {
  token = "internal-token-value"
}`,
			},
			expectedEnvVars:          map[string]string{},
			shouldHaveProviderMirror: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tempDir := t.TempDir()

			// Configure Terraform registry
			ctx := context.Background()
			regConfig, err := ConfigureTerraformRegistry(ctx, tt.config, tt.secrets, tempDir)

			require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")
			require.NotNil(t, regConfig, "Registry config should not be nil")

			// Read the generated .terraformrc file
			terraformRCPath := filepath.Join(tempDir, TerraformRCFilename)
			require.FileExists(t, terraformRCPath, ".terraformrc file should be created")

			content, err := os.ReadFile(terraformRCPath)
			require.NoError(t, err, "Should be able to read .terraformrc file")

			configContent := string(content)
			t.Logf("Generated .terraformrc content for test '%s':\n%s", tt.name, configContent)

			// Verify provider installation block presence
			if tt.shouldHaveProviderMirror {
				require.Contains(t, configContent, "provider_installation {", "Should contain provider installation block")
				require.Contains(t, configContent, "network_mirror {", "Should contain network mirror block")
			} else {
				require.NotContains(t, configContent, "provider_installation {", "Should not contain provider installation block")
			}

			// Verify host blocks for module registry redirection
			for _, expectedHost := range tt.expectedHostBlocks {
				require.Contains(t, configContent, expectedHost, "Should contain expected host block")
			}

			// Verify credentials blocks
			for _, expectedCred := range tt.expectedCredentials {
				require.Contains(t, configContent, expectedCred, "Should contain expected credentials block")
			}

			// Verify environment variables
			for expectedVar, expectedValue := range tt.expectedEnvVars {
				actualValue, exists := regConfig.EnvVars[expectedVar]
				require.True(t, exists, "Environment variable %s should be set", expectedVar)
				require.Equal(t, expectedValue, actualValue, "Environment variable %s should have correct value", expectedVar)
			}

			// Verify TF_CLI_CONFIG_FILE is set
			require.Equal(t, terraformRCPath, regConfig.EnvVars[EnvTerraformCLIConfigFile], "TF_CLI_CONFIG_FILE should be set to .terraformrc path")

			// Cleanup
			err = CleanupTerraformRegistryConfig(ctx, regConfig)
			require.NoError(t, err, "Cleanup should not return an error")
		})
	}
}

func TestGetTerraformTokenEnv_GitLabAirGapped(t *testing.T) {
	// Test the token environment variable generation for GitLab air-gapped hostnames
	tests := []struct {
		name        string
		hostname    string
		token       string
		expectedVar string
		expectedVal string
	}{
		{
			name:     "Air-gapped hostname with port",
			hostname: "providermirror.example.com:5443",
			// Example token shape that does not match real GitLab PAT patterns
			token:       "example-test-token",
			expectedVar: "TF_TOKEN_providermirror_example_com_5443",
			expectedVal: "example-test-token",
		},
		{
			name:        "registry.terraform.io hostname",
			hostname:    "registry.terraform.io",
			token:       "registry-token",
			expectedVar: "TF_TOKEN_registry_terraform_io",
			expectedVal: "registry-token",
		},
		{
			name:        "simple hostname without port",
			hostname:    "providermirror.example.com",
			token:       "example-simple-token",
			expectedVar: "TF_TOKEN_providermirror_example_com",
			expectedVal: "example-simple-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			envVar, envVal, err := getTerraformTokenEnv(tt.hostname, tt.token)

			require.NoError(t, err, "Should not return an error")
			require.Equal(t, tt.expectedVar, envVar, "Environment variable name should match")
			require.Equal(t, tt.expectedVal, envVal, "Environment variable value should match")
		})
	}
}

func TestConfigureTerraformRegistry_GitAuthentication(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		registryHost    = "providermirror.example.com"
		expectedGitHost = "providermirror.example.com"
		secretStoreID   = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitSecret"
		// Example token for tests; does not match real PAT patterns
		token = "example-gitlab-token"
	)

	// Setup configuration with module registry that requires Git authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ModuleRegistries: map[string]*datamodel.TerraformModuleRegistryConfig{
					"gitlab": {
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

	// Verify .gitconfig exists and has the correct content
	gitConfigPath := filepath.Join(tempDir, ".gitconfig")
	require.FileExists(t, gitConfigPath, ".gitconfig should be created")
	gitCfgContent, err := os.ReadFile(gitConfigPath)
	require.NoError(t, err, "Should be able to read .gitconfig")
	gitCfgStr := string(gitCfgContent)

	// Verify URL rewriting
	expectedURLSection := fmt.Sprintf("[url \"https://%s/\"]", expectedGitHost)
	require.Contains(t, gitCfgStr, expectedURLSection, ".gitconfig should contain url section for host")
	require.Contains(t, gitCfgStr, "insteadOf = https://github.com/", ".gitconfig should contain insteadOf rule")

	// Verify authentication header
	expectedHeader := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("oauth2:"+token))
	require.Contains(t, gitCfgStr, fmt.Sprintf("[http \"https://%s\"]", expectedGitHost), ".gitconfig should contain http section for host")
	require.Contains(t, gitCfgStr, expectedHeader, ".gitconfig should configure Basic Authorization header")

	// Verify file permissions
	gitCfgStat, err := os.Stat(gitConfigPath)
	require.NoError(t, err, "Should be able to stat .gitconfig file")
	require.Equal(t, os.FileMode(0600), gitCfgStat.Mode().Perm(), ".gitconfig should be 0600")

	// Verify Git environment variables are set
	require.Contains(t, regConfig.EnvVars, "GIT_CONFIG_GLOBAL", "Should set GIT_CONFIG_GLOBAL environment variable")
	require.Equal(t, gitConfigPath, regConfig.EnvVars["GIT_CONFIG_GLOBAL"], "GIT_CONFIG_GLOBAL should point to the generated .gitconfig")
	require.Contains(t, regConfig.EnvVars, "HOME", "Should set HOME environment variable")
	require.Equal(t, tempDir, regConfig.EnvVars["HOME"], "HOME should be set to working directory")
	require.Equal(t, "0", regConfig.EnvVars["GIT_TERMINAL_PROMPT"], "Should disable interactive Git prompts")

	// Verify temp files tracked
	require.Contains(t, regConfig.TempFiles, gitConfigPath, ".gitconfig should be tracked for cleanup")

	// Verify Terraform credentials are also configured
	terraformRCContent, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read .terraformrc file")
	configContent := string(terraformRCContent)
	require.Contains(t, configContent, fmt.Sprintf(`credentials "%s"`, registryHost), ".terraformrc should contain credentials block for registry")
	require.Contains(t, configContent, fmt.Sprintf(`token = "%s"`, token), ".terraformrc should contain the token")

	// Test cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err, "Cleanup should not return an error")
	require.NoFileExists(t, gitConfigPath, ".gitconfig should be removed after cleanup")
}

func TestConfigureTerraformRegistry_ProviderMirror_Network_NoAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Type: "network",
					URL:  "https://mirror.example.com/providers/mirror/",
				},
			},
		},
	}

	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.NoError(t, err)
	require.NotNil(t, regConfig)

	cfgPath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, cfgPath)
	b, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	content := string(b)

	require.Contains(t, content, "provider_installation {")
	require.Contains(t, content, "network_mirror {")
	// URL in config trims trailing slash
	require.Contains(t, content, `url = "https://mirror.example.com/providers/mirror"`)
	require.Contains(t, content, "direct {}")

	// Only TF_CLI_CONFIG_FILE should be set
	require.Len(t, regConfig.EnvVars, 1)
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile)
}

func TestConfigureTerraformRegistry_ProviderMirror_Network_WithCACert_SetsAllCAEnvs(t *testing.T) {
	tempDir := t.TempDir()

	const (
		caSecretID   = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/certs"
		caSecretKey  = "ca.crt"
		caSecretData = "-----BEGIN CERTIFICATE-----\nMIID...test...\n-----END CERTIFICATE-----\n"
	)

	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Type: "network",
					URL:  "https://mirror.example.com/providers/mirror/",
					TLS: &datamodel.TLSConfig{
						CACertificate: &datamodel.SecretReference{Source: caSecretID, Key: caSecretKey},
					},
				},
			},
		},
	}
	secrets := map[string]recipes.SecretData{
		caSecretID: {Type: "opaque", Data: map[string]string{caSecretKey: caSecretData}},
	}

	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err)

	// Provider mirrors do not support TLS - no CA environment variables should be set
	_, hasSSL := regConfig.EnvVars["SSL_CERT_FILE"]
	require.False(t, hasSSL, "SSL_CERT_FILE should not be set for provider mirrors")
	_, hasCURL := regConfig.EnvVars["CURL_CA_BUNDLE"]
	require.False(t, hasCURL, "CURL_CA_BUNDLE should not be set for provider mirrors")
	_, hasGit := regConfig.EnvVars["GIT_SSL_CAINFO"]
	require.False(t, hasGit, "GIT_SSL_CAINFO should not be set for provider mirrors")

	// No certificate files should be created
	require.Len(t, regConfig.TempFiles, 0, "No certificate files should be created for provider mirrors")

	// Config should be network_mirror
	b, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err)
	content := string(b)
	require.Contains(t, content, "network_mirror {")
}

func TestConfigureTerraformRegistry_ProviderMirror_UnsupportedType(t *testing.T) {
	tempDir := t.TempDir()

	// Test that the deprecated type field is ignored (no error should occur)
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				ProviderMirror: &datamodel.TerraformProviderMirrorConfig{
					Type: "s3", // This deprecated field should be ignored
					URL:  "https://mirror.example.com/providers/",
				},
			},
		},
	}
	ctx := context.Background()
	regConfig, err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.NoError(t, err, "Should not return error - type field is deprecated and ignored")
	require.NotNil(t, regConfig, "Should return valid registry config")

	// Verify network mirror is used regardless of the deprecated type field
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err)
	configContent := string(content)
	require.Contains(t, configContent, "network_mirror {", "Should use network mirror regardless of deprecated type field")
}
