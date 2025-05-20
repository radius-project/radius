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
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// TerraformRCFilename is the filename for Terraform registry configuration
	TerraformRCFilename = ".terraformrc"

	// EnvTerraformCLIConfigFile is the environment variable used to specify the location of Terraform config file
	EnvTerraformCLIConfigFile = "TF_CLI_CONFIG_FILE"

	// DefaultFilePerms defines secure file permissions for the Terraform config file (0600 = owner read/write only)
	DefaultFilePerms = 0600
)

// ConfigureTerraformRegistry sets up Terraform registry configuration for private registries.
// It creates a .terraformrc file with the registry mirror and authentication details.
func ConfigureTerraformRegistry(ctx context.Context, config recipes.Configuration, secrets map[string]recipes.SecretData, dirPath string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Check if registry mirror configuration exists
	if config.RecipeConfig.Terraform.Registry == nil || config.RecipeConfig.Terraform.Registry.Mirror == "" {
		logger.Info("No Terraform registry mirror configured, skipping registry configuration")
		return nil
	}

	// Extract and validate the mirror URL
	mirrorURL := config.RecipeConfig.Terraform.Registry.Mirror

	// Validate the URL is well-formed
	parsedURL, err := url.Parse(mirrorURL)
	if err != nil {
		return fmt.Errorf("invalid terraform registry mirror URL: %w", err)
	}

	// Security warning for non-localhost HTTP URLs
	if parsedURL.Scheme == "http" && !strings.Contains(parsedURL.Host, "localhost") && !strings.Contains(parsedURL.Host, "127.0.0.1") {
		logger.Info("Using insecure HTTP for Terraform registry mirror in production environment is not recommended")
	}

	// Use the extractHostname helper function instead of direct access to parsedURL.Hostname()
	hostname, err := extractHostname(mirrorURL)
	if err != nil {
		return fmt.Errorf("could not extract hostname from mirror URL: %w", err)
	}

	if hostname == "" {
		return fmt.Errorf("empty hostname extracted from mirror URL: %s", mirrorURL)
	}

	// Begin building Terraform configuration
	var configContent strings.Builder

	// Track if authentication is configured
	hasAuth := false

	// Handle token-based authentication
	auth := config.RecipeConfig.Terraform.Registry.Authentication
	if auth.Token != nil && auth.Token.Source != "" {
		tokenRef := auth.Token
		secretStoreID := tokenRef.Source
		secretKey := tokenRef.Key

		// Get token from secret store
		if secrets != nil {
			if secretData, ok := secrets[secretStoreID]; ok {
				if token, ok := secretData.Data[secretKey]; ok {
					configContent.WriteString(fmt.Sprintf("credentials %q {\n  token = %q\n}\n\n", hostname, token))
					hasAuth = true
					os.Setenv("TF_MIRROR_TOKEN", string(token))
				} else {
					logger.Info("Secret key not found in secret store",
						"secretKey", secretKey,
						"secretStoreID", secretStoreID)
				}
			} else {
				logger.Info("Secret store not found", "secretStoreID", secretStoreID)
			}
		}
	}

	// Configure provider mappings if present
	if len(config.RecipeConfig.Terraform.Registry.ProviderMappings) > 0 {
		configContent.WriteString("provider_source_overrides {\n")
		for source, replacement := range config.RecipeConfig.Terraform.Registry.ProviderMappings {
			configContent.WriteString(fmt.Sprintf("  %q = %q\n", source, replacement))
		}
		configContent.WriteString("}\n\n")
	}

	// Add provider installation configuration with mirror URL
	configContent.WriteString(fmt.Sprintf("provider_installation {\n  network_mirror {\n    url = %q\n  }\n}\n", mirrorURL))

	// Create the .terraformrc file in the execution directory
	terraformRCPath := filepath.Join(dirPath, TerraformRCFilename)
	err = os.WriteFile(terraformRCPath, []byte(configContent.String()), DefaultFilePerms)
	if err != nil {
		return fmt.Errorf("failed to write Terraform registry configuration file: %w", err)
	}

	// Set the TF_CLI_CONFIG_FILE environment variable to point to our config file
	os.Setenv(EnvTerraformCLIConfigFile, terraformRCPath)

	logger.Info("Terraform registry configuration created",
		"path", terraformRCPath,
		"mirror", mirrorURL,
		"hasAuth", hasAuth,
		"hasProviderMappings", len(config.RecipeConfig.Terraform.Registry.ProviderMappings) > 0)

	return nil
}

// extractHostname gets the hostname part from a URL
func extractHostname(urlStr string) (string, error) {
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}

	return parsedURL.Hostname(), nil
}

// CleanupTerraformRegistryConfig removes the Terraform registry configuration and unsets environment variables
func CleanupTerraformRegistryConfig(ctx context.Context, dirPath string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Unset the TF_CLI_CONFIG_FILE environment variable
	os.Unsetenv(EnvTerraformCLIConfigFile)

	// Check if the file exists and log its presence/absence
	configPath := filepath.Join(dirPath, TerraformRCFilename)
	if _, err := os.Stat(configPath); err == nil {
		logger.Info("Cleaned up Terraform registry configuration", "path", configPath)
	} else {
		logger.Info("No Terraform registry configuration to clean up")
	}

	return nil
}
