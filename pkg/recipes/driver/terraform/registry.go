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

// RegistryConfig tracks the configuration created for cleanup
type RegistryConfig struct {
	ConfigPath string
	EnvVars    []string
}

// ConfigureTerraformRegistry sets up Terraform registry configuration for private registries.
// It creates a .terraformrc file with the registry mirror and sets up authentication via environment variables.
// Returns a RegistryConfig struct that tracks created resources for cleanup.
func ConfigureTerraformRegistry(ctx context.Context, config recipes.Configuration, secrets map[string]recipes.SecretData, dirPath string) (*RegistryConfig, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize the registry config to track what we create
	regConfig := &RegistryConfig{
		EnvVars: []string{},
	}

	// Check if registry mirror configuration exists
	if config.RecipeConfig.Terraform.Registry == nil || config.RecipeConfig.Terraform.Registry.Mirror == "" {
		logger.Info("No Terraform registry mirror configured, skipping registry configuration")
		return nil, nil
	}

	// Extract and validate the mirror URL
	mirrorURL := config.RecipeConfig.Terraform.Registry.Mirror
	logger.Info("Configuring Terraform registry with mirror", "url", mirrorURL)

	// Check if URL is malformed first (e.g., starts with ://)
	if strings.HasPrefix(mirrorURL, "://") {
		return nil, fmt.Errorf("invalid terraform registry mirror URL: %s", mirrorURL)
	}

	// Try parsing the URL as-is
	parsedURL, err := url.Parse(mirrorURL)
	if err != nil {
		return nil, fmt.Errorf("invalid terraform registry mirror URL: %w", err)
	}

	// If no scheme is present, add https:// and reparse
	// This handles cases like "example.com" or "example.com:8443"
	if parsedURL.Scheme == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && strings.Contains(mirrorURL, ":")) {
		// For URLs without a scheme, Go's url.Parse may misinterpret the host as scheme
		// e.g., "example.com:8443" becomes scheme="example.com" instead of host="example.com:8443"
		// So we always add https:// for scheme-less URLs
		mirrorURL = "https://" + mirrorURL
		parsedURL, err = url.Parse(mirrorURL)
		if err != nil {
			return nil, fmt.Errorf("invalid terraform registry mirror URL: %w", err)
		}
	}

	// Use Host() instead of Hostname() to preserve port information
	host := parsedURL.Host
	if host == "" {
		return nil, fmt.Errorf("empty host in mirror URL: %s", config.RecipeConfig.Terraform.Registry.Mirror)
	}

	// Begin building Terraform configuration
	var configContent strings.Builder

	// Handle authentication
	auth := config.RecipeConfig.Terraform.Registry.Authentication

	// Token authentication
	if auth.Token != nil && auth.Token.Secret != "" {
		secretStoreID := auth.Token.Secret

		// Get token from secret store
		if secrets == nil {
			return nil, fmt.Errorf("no secrets available for token authentication")
		}

		secretData, secretExists := secrets[secretStoreID]
		if !secretExists {
			return nil, fmt.Errorf("secret store %q not found", secretStoreID)
		}

		token, tokenExists := secretData.Data["token"]
		if !tokenExists {
			return nil, fmt.Errorf("token not found in secret store %q", secretStoreID)
		}

		// Set environment variable directly with the token (no encoding)
		envVar := setTerraformTokenEnv(host, string(token))
		regConfig.EnvVars = append(regConfig.EnvVars, envVar)

		logger.Info("Configured token authentication",
			"host", host,
			"secretStoreID", secretStoreID)

		// Handle additional hosts if configured
		if len(auth.AdditionalHosts) > 0 {
			logger.Info("Configuring authentication for additional hosts", "hosts", auth.AdditionalHosts)

			// Apply same token to each additional host
			for _, additionalHost := range auth.AdditionalHosts {
				if additionalHost == "" || additionalHost == host {
					continue // Skip empty or duplicate hosts
				}

				// Set environment variable
				envVar := setTerraformTokenEnv(additionalHost, string(token))
				regConfig.EnvVars = append(regConfig.EnvVars, envVar)
				
				logger.Info("Added token authentication for additional host", "host", additionalHost)
			}
		}
	}

	// Add provider installation configuration with normalized mirror URL
	configContent.WriteString(fmt.Sprintf(`provider_installation {
  network_mirror {
    url     = %q
    include = ["*/*/*"]
  }
  direct {
    exclude = ["*/*/*"]
  }
}
`, parsedURL.String()))

	// Create the .terraformrc file in the execution directory
	terraformRCPath := filepath.Join(dirPath, TerraformRCFilename)
	logger.Info("Writing Terraform registry configuration file", "path", terraformRCPath)

	err = os.WriteFile(terraformRCPath, []byte(configContent.String()), DefaultFilePerms)
	if err != nil {
		return nil, fmt.Errorf("failed to write Terraform registry configuration file: %w", err)
	}
	regConfig.ConfigPath = terraformRCPath

	// Set the TF_CLI_CONFIG_FILE environment variable to point to our config file
	os.Setenv(EnvTerraformCLIConfigFile, terraformRCPath)
	regConfig.EnvVars = append(regConfig.EnvVars, EnvTerraformCLIConfigFile)

	logger.Info("Set environment variable for Terraform config",
		"variable", EnvTerraformCLIConfigFile,
		"value", terraformRCPath)

	logger.Info("Terraform registry configuration created",
		"path", terraformRCPath,
		"mirror", parsedURL.String())

	return regConfig, nil
}

// setTerraformTokenEnv sets the TF_TOKEN_* environment variable for a hostname
// Returns the environment variable name for tracking
func setTerraformTokenEnv(hostname string, token string) string {
	// Replace dots and colons with underscores in hostname (for ports)
	envHostname := strings.ReplaceAll(hostname, ".", "_")
	envHostname = strings.ReplaceAll(envHostname, ":", "_")
	envVarName := fmt.Sprintf("TF_TOKEN_%s", envHostname)
	os.Setenv(envVarName, token)
	return envVarName
}

// CleanupTerraformRegistryConfig removes the Terraform registry configuration and unsets environment variables
func CleanupTerraformRegistryConfig(ctx context.Context, config *RegistryConfig) error {
	if config == nil {
		return nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)

	// Unset all tracked environment variables
	for _, envVar := range config.EnvVars {
		os.Unsetenv(envVar)
		logger.Info("Unset environment variable", "variable", envVar)
	}

	// Remove the config file if it exists
	if config.ConfigPath != "" {
		if err := os.Remove(config.ConfigPath); err != nil && !os.IsNotExist(err) {
			logger.Info("Failed to remove Terraform config file", "path", config.ConfigPath, "error", err)
			// Don't return error as this is just cleanup
		}
	}

	return nil
}
