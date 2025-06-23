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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/require"
)

func TestConfigureTerraformRegistry(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Test registry configuration
	const (
		mirrorURL     = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		token         = "test-auth-token-12345"
	)

	// Setup configuration from the example
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					ProviderMappings: map[string]string{
						"hashicorp/azurerm": "mycompany/azurerm",
					},
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
	require.NotNil(t, regConfig, "Should return a RegistryConfig")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")
	require.Equal(t, configFilePath, regConfig.ConfigPath, "RegistryConfig should contain the correct config path")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content does NOT contain credentials (only env vars now)
	require.False(t, strings.Contains(configContent, "credentials"),
		"Config file should NOT contain credentials block")

	// Check for provider installation block with normalized mirror URL
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, `url     = "https://terraform.example.com"`),
		"Config file should contain the normalized mirror URL with https scheme")
	require.True(t, strings.Contains(configContent, `include = ["*/*/*"]`),
		"Config file should contain include pattern")
	require.True(t, strings.Contains(configContent, `exclude = ["*/*/*"]`),
		"Config file should contain exclude pattern")

	// Verify environment variable was set
	require.Equal(t, configFilePath, os.Getenv(EnvTerraformCLIConfigFile),
		"TF_CLI_CONFIG_FILE environment variable should be set")

	// Verify TF_TOKEN_* environment variable was set with raw token value
	require.Equal(t, token, os.Getenv("TF_TOKEN_terraform_example_com"),
		"TF_TOKEN_* environment variable should be set with raw token value")

	// Verify the RegistryConfig tracks the environment variables
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile,
		"RegistryConfig should track TF_CLI_CONFIG_FILE")
	require.Contains(t, regConfig.EnvVars, "TF_TOKEN_terraform_example_com",
		"RegistryConfig should track TF_TOKEN_* variable")

	// Test cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err, "Cleanup should not return an error")
	require.Empty(t, os.Getenv(EnvTerraformCLIConfigFile),
		"TF_CLI_CONFIG_FILE should be unset after cleanup")
	require.Empty(t, os.Getenv("TF_TOKEN_terraform_example_com"),
		"TF_TOKEN_* should be unset after cleanup")
}

func TestConfigureTerraformRegistry_NoAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Setup configuration with mirror but no auth
	const mirrorURL = "terraform.example.com"
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
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
	require.Equal(t, EnvTerraformCLIConfigFile, regConfig.EnvVars[0],
		"Should only track TF_CLI_CONFIG_FILE")
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

func TestConfigureTerraformRegistry_WithPort(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com:8443"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		token         = "test-token-with-port"
	)

	// Setup configuration with port in URL
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
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
	require.Equal(t, token, os.Getenv("TF_TOKEN_terraform_example_com_8443"),
		"TF_TOKEN_* should include port with colons replaced by underscores")

	// Read the generated file
	content, err := os.ReadFile(regConfig.ConfigPath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the normalized URL includes the port
	require.True(t, strings.Contains(configContent, `url     = "https://terraform.example.com:8443"`),
		"Config file should contain the normalized mirror URL with port")

	// Cleanup
	require.NoError(t, os.Unsetenv("TF_TOKEN_terraform_example_com_8443"))
}

func TestConfigureTerraformRegistry_MissingToken(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
	)

	// Setup configuration with token authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
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

func TestConfigureTerraformRegistry_AdditionalHosts(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "https://my-registry.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecrets"
		token         = "test-token-12345"
	)

	// Setup configuration with additional hosts
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Token: &datamodel.TokenConfig{
							Secret: secretStoreID,
						},
						AdditionalHosts: []string{"original-registry.example.com", "backup-registry.example.com"},
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

	// Verify environment variables for all hosts
	
	// Check primary host
	require.Equal(t, token, os.Getenv("TF_TOKEN_my-registry_example_com"),
		"TF_TOKEN_* should be set for primary host")
	
	// Check additional hosts
	require.Equal(t, token, os.Getenv("TF_TOKEN_original-registry_example_com"),
		"TF_TOKEN_* should be set for first additional host")
	require.Equal(t, token, os.Getenv("TF_TOKEN_backup-registry_example_com"),
		"TF_TOKEN_* should be set for second additional host")

	// Verify all env vars are tracked
	require.Len(t, regConfig.EnvVars, 4, "Should track all environment variables")
	require.Contains(t, regConfig.EnvVars, EnvTerraformCLIConfigFile)
	require.Contains(t, regConfig.EnvVars, "TF_TOKEN_my-registry_example_com")
	require.Contains(t, regConfig.EnvVars, "TF_TOKEN_original-registry_example_com")
	require.Contains(t, regConfig.EnvVars, "TF_TOKEN_backup-registry_example_com")

	// Cleanup
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)
	
	// Verify all env vars are cleaned up
	require.Empty(t, os.Getenv("TF_TOKEN_my-registry_example_com"))
	require.Empty(t, os.Getenv("TF_TOKEN_original-registry_example_com"))
	require.Empty(t, os.Getenv("TF_TOKEN_backup-registry_example_com"))
}

func TestConfigureTerraformRegistry_InvalidURL(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	// Setup configuration with invalid URL
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
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
		EnvVars:    []string{"TEST_VAR"},
	}
	
	// Set a test env var
	require.NoError(t, os.Setenv("TEST_VAR", "test-value"))
	
	// Call cleanup
	ctx := context.Background()
	err = CleanupTerraformRegistryConfig(ctx, regConfig)
	require.NoError(t, err)
	
	// Verify file is removed
	_, err = os.Stat(configPath)
	require.True(t, os.IsNotExist(err), "Config file should be removed")
	
	// Verify env var is unset
	require.Empty(t, os.Getenv("TEST_VAR"), "Environment variable should be unset")
}