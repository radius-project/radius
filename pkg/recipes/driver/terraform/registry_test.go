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
						Token: &datamodel.SecretReference{
							Source: secretStoreID,
							Key:    secretKey,
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
				secretKey: tokenValue,
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
  token = "test-secret-token-value"
}`
	require.True(t, strings.Contains(configContent, expectedCredentials),
		"Config file should contain credentials block with the token")

	expectedMirror := `provider_installation {
  network_mirror {
    url = "terraform.example.com"
  }
}`
	require.True(t, strings.Contains(configContent, expectedMirror),
		"Config file should contain the provider installation block with mirror URL")

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

	expectedMirror := `provider_installation {
  network_mirror {
    url = "terraform.example.com"
  }
}`
	require.True(t, strings.Contains(configContent, expectedMirror),
		"Config file should contain the provider installation block with mirror URL")
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
