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
		secretKey     = "registryToken"
		tokenValue    = "test-secret-token-value"
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
						Basic: &datamodel.BasicAuthConfig{
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
				"username": "testuser",
				"password": tokenValue,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content contains the expected sections
	expectedCredentials := `credentials "terraform.example.com" {
  username = "testuser"
  password = "test-secret-token-value"
}`
	require.True(t, strings.Contains(configContent, expectedCredentials),
		"Config file should contain credentials block with username and password")

	// Check for provider installation block with mirror URL
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, `url    = "terraform.example.com"`),
		"Config file should contain the mirror URL")
	require.True(t, strings.Contains(configContent, `include = ["*/*/*"]`),
		"Config file should contain include pattern")
	require.True(t, strings.Contains(configContent, `exclude = ["*/*/*"]`),
		"Config file should contain exclude pattern")

	// Verify environment variable was set
	require.Equal(t, configFilePath, os.Getenv(EnvTerraformCLIConfigFile),
		"TF_CLI_CONFIG_FILE environment variable should be set")

	// Test cleanup
	err = CleanupTerraformRegistryConfig(ctx, tempDir)
	require.NoError(t, err, "Cleanup should not return an error")
	require.Empty(t, os.Getenv(EnvTerraformCLIConfigFile),
		"Environment variable should be unset after cleanup")
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
	err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

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
	require.True(t, strings.Contains(configContent, `url    = "terraform.example.com"`),
		"Config file should contain the mirror URL")
	require.True(t, strings.Contains(configContent, `include = ["*/*/*"]`),
		"Config file should contain include pattern")
	require.True(t, strings.Contains(configContent, `exclude = ["*/*/*"]`),
		"Config file should contain exclude pattern")
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
	err := ConfigureTerraformRegistry(ctx, config, nil, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was NOT created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	_, err = os.Stat(configFilePath)
	require.True(t, os.IsNotExist(err), "No .terraformrc file should be created when no registry is configured")
}

func TestConfigureTerraformRegistry_BasicAuthWithUsername(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		password      = "github_pat_test123"
		username      = "testuser"
	)

	// Setup configuration with basic authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Basic: &datamodel.BasicAuthConfig{
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
				"password": password,
				"username": username,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content contains the expected credentials
	expectedCredentials := `credentials "terraform.example.com" {
  username = "testuser"
  password = "github_pat_test123"
}`
	require.True(t, strings.Contains(configContent, expectedCredentials),
		"Config file should contain credentials block with username and password")
}

func TestConfigureTerraformRegistry_BasicAuth(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
		username      = "basicuser"
		password      = "basicpass123"
	)

	// Setup configuration with basic authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Basic: &datamodel.BasicAuthConfig{
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
				"username": username,
				"password": password,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content contains the expected credentials
	expectedCredentials := `credentials "terraform.example.com" {
  username = "basicuser"
  password = "basicpass123"
}`
	require.True(t, strings.Contains(configContent, expectedCredentials),
		"Config file should contain credentials block with username and password")
}

// Removing CustomHeaders test as this auth type is no longer supported
/*
func TestConfigureTerraformRegistry_CustomHeaders(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL      = "terraform.example.com"
		secretStoreID1 = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore1"
		secretStoreID2 = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore2"
		apiKeyValue    = "api-key-123"
		authTokenValue = "bearer-token-456"
	)

	// Setup configuration with custom headers
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						CustomHeaders: map[string]datamodel.SecretReference{
							"X-API-Key": {
								Source: secretStoreID1,
								Key:    "apiKey",
							},
							"Authorization": {
								Source: secretStoreID2,
								Key:    "token",
							},
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data
	secrets := map[string]recipes.SecretData{
		secretStoreID1: {
			Type: "opaque",
			Data: map[string]string{
				"apiKey": apiKeyValue,
			},
		},
		secretStoreID2: {
			Type: "opaque",
			Data: map[string]string{
				"token": authTokenValue,
			},
		},
	}

	// Clear any existing environment variables
	os.Unsetenv("TF_REGISTRY_HEADER_X_API_KEY")
	os.Unsetenv("TF_REGISTRY_HEADER_AUTHORIZATION")

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Clean up environment variables
	os.Unsetenv("TF_REGISTRY_HEADER_X_API_KEY")
	os.Unsetenv("TF_REGISTRY_HEADER_AUTHORIZATION")
}
*/

// Removing ClientCertificate test as this auth type is no longer supported
/*
func TestConfigureTerraformRegistry_ClientCertificate(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
	)

	// Setup configuration with client certificate
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						ClientCertificate: &datamodel.ClientCertConfig{
							Secret: secretStoreID,
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data (not used directly, but needed for validation)
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"certificate": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
				"key":         "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----",
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was created (even though client cert is handled differently)
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// The actual client certificate handling would be done at the HTTP transport level
	// This test just verifies that the configuration is accepted
}
*/

func TestConfigureTerraformRegistry_MissingSecrets(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "terraform.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecretStore"
	)

	// Setup configuration with basic authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Basic: &datamodel.BasicAuthConfig{
							Secret: secretStoreID,
						},
					},
				},
			},
		},
	}

	// Setup mock secrets data with missing username
	secrets := map[string]recipes.SecretData{
		secretStoreID: {
			Type: "opaque",
			Data: map[string]string{
				"password": "pass123", // username is missing
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.Error(t, err, "ConfigureTerraformRegistry should return an error when required secrets are missing")
	require.Contains(t, err.Error(), "username or password not found")
}

func TestConfigureTerraformRegistry_GitLabPagesMirror(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "https://ytimocin-group.gitlab.io/terraform-registry/"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/gitlabSecrets"
		patValue      = "glpat-xxxxxxxxxxxx"
	)

	// Setup configuration with GitLab Pages mirror and basic authentication
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Basic: &datamodel.BasicAuthConfig{
							Secret: secretStoreID,
						},
						AdditionalHosts: []string{"gitlab.com"},
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
				"username": "oauth2",
				"password": patValue,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content contains credentials for both the mirror host and gitlab.com
	expectedMirrorCredentials := `credentials "ytimocin-group.gitlab.io" {
  username = "oauth2"
  password = "glpat-xxxxxxxxxxxx"
}`
	require.True(t, strings.Contains(configContent, expectedMirrorCredentials),
		"Config file should contain credentials block for mirror host")

	expectedGitLabCredentials := `credentials "gitlab.com" {
  username = "oauth2"
  password = "glpat-xxxxxxxxxxxx"
}`
	require.True(t, strings.Contains(configContent, expectedGitLabCredentials),
		"Config file should contain credentials block for gitlab.com")

	// Check for provider installation block with mirror URL
	require.True(t, strings.Contains(configContent, "provider_installation {"),
		"Config file should contain provider_installation block")
	require.True(t, strings.Contains(configContent, `url    = "https://ytimocin-group.gitlab.io/terraform-registry/"`),
		"Config file should contain the mirror URL")
}

func TestConfigureTerraformRegistry_AdditionalHosts(t *testing.T) {
	// Create temp dir for test
	tempDir := t.TempDir()

	const (
		mirrorURL     = "https://my-registry.example.com"
		secretStoreID = "/planes/radius/local/resourcegroups/mygroup/providers/Applications.Core/secretStores/mySecrets"
		tokenValue    = "test-token-12345"
	)

	// Setup configuration with additional hosts
	config := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Terraform: datamodel.TerraformConfigProperties{
				Registry: &datamodel.TerraformRegistryConfig{
					Mirror: mirrorURL,
					Authentication: datamodel.RegistryAuthConfig{
						Basic: &datamodel.BasicAuthConfig{
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
				"username": "testuser",
				"password": tokenValue,
			},
		},
	}

	// Call the function under test
	ctx := context.Background()
	err := ConfigureTerraformRegistry(ctx, config, secrets, tempDir)
	require.NoError(t, err, "ConfigureTerraformRegistry should not return an error")

	// Verify the .terraformrc file was created
	configFilePath := filepath.Join(tempDir, TerraformRCFilename)
	require.FileExists(t, configFilePath, ".terraformrc file should be created")

	// Read the generated file
	content, err := os.ReadFile(configFilePath)
	require.NoError(t, err, "Should be able to read the config file")
	configContent := string(content)

	// Verify the content contains credentials for the mirror host
	expectedMirrorCredentials := `credentials "my-registry.example.com" {
  username = "testuser"
  password = "test-token-12345"
}`
	require.True(t, strings.Contains(configContent, expectedMirrorCredentials),
		"Config file should contain credentials block for mirror host")

	// Verify the content contains credentials for additional hosts
	expectedAdditionalHost1 := `credentials "original-registry.example.com" {
  username = "testuser"
  password = "test-token-12345"
}`
	require.True(t, strings.Contains(configContent, expectedAdditionalHost1),
		"Config file should contain credentials block for first additional host")

	expectedAdditionalHost2 := `credentials "backup-registry.example.com" {
  username = "testuser"
  password = "test-token-12345"
}`
	require.True(t, strings.Contains(configContent, expectedAdditionalHost2),
		"Config file should contain credentials block for second additional host")
}
