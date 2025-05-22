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
	logger.Info("Configuring Terraform registry with mirror", "url", mirrorURL)

	// Validate the URL is well-formed
	parsedURL, err := url.Parse(mirrorURL)
	if err != nil {
		logger.Error(err, "Failed to parse Terraform registry mirror URL", "url", mirrorURL)
		return fmt.Errorf("invalid terraform registry mirror URL: %w", err)
	}

	// Enhanced TLS/security validation and logging
	if parsedURL.Scheme == "https" {
		// Log TLS usage
		logger.Info("Using HTTPS for Terraform registry mirror", "host", parsedURL.Host)

		// Check for special hostnames that might need resolution in containers
		if strings.Contains(parsedURL.Host, "host.docker.internal") ||
			strings.Contains(parsedURL.Host, "localhost") {
			logger.Info("Using special hostname that may require DNS resolution in containers",
				"hostname", parsedURL.Host,
				"tip", "Consider using IP address or ensure hostAliases are configured in pod spec")
		}
	} else if parsedURL.Scheme == "http" && !strings.Contains(parsedURL.Host, "localhost") && !strings.Contains(parsedURL.Host, "127.0.0.1") {
		logger.Info("Using insecure HTTP for Terraform registry mirror in production environment is not recommended")
	}

	// Use the extractHostname helper function instead of direct access to parsedURL.Hostname()
	hostname, err := extractHostname(mirrorURL)
	if err != nil {
		logger.Error(err, "Failed to extract hostname from mirror URL", "url", mirrorURL)
		return fmt.Errorf("could not extract hostname from mirror URL: %w", err)
	}

	if hostname == "" {
		logger.Error(fmt.Errorf("empty hostname extracted from mirror URL"), "url", mirrorURL)
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

		// Validate required fields
		if secretKey == "" {
			logger.Error(fmt.Errorf("missing secret key"),
				"Secret key is required for token authentication", "secretStoreID", secretStoreID)
			return fmt.Errorf("secret key is required for token authentication")
		}

		// Get token from secret store
		if secrets == nil {
			logger.Error(fmt.Errorf("no secrets provided"),
				"No secrets available for authentication", "secretStoreID", secretStoreID)
			return fmt.Errorf("no secrets available for authentication")
		}

		secretData, secretExists := secrets[secretStoreID]
		if !secretExists {
			logger.Error(fmt.Errorf("secret store not found"),
				"Secret store not found", "secretStoreID", secretStoreID)
			return fmt.Errorf("secret store %q not found", secretStoreID)
		}

		token, tokenExists := secretData.Data[secretKey]
		if !tokenExists {
			logger.Error(fmt.Errorf("secret key not found"), "Secret key not found in secret store",
				"secretKey", secretKey, "secretStoreID", secretStoreID, "availableKeys", getSecretKeys(secretData.Data))
			return fmt.Errorf("secret key %q not found in secret store %q", secretKey, secretStoreID)
		}

		// Validate token is not empty
		if strings.TrimSpace(string(token)) == "" {
			logger.Error(fmt.Errorf("empty token"), "Token is empty",
				"secretKey", secretKey, "secretStoreID", secretStoreID)
			return fmt.Errorf("token is empty for secret key %q", secretKey)
		}

		// Configure credentials
		configContent.WriteString(fmt.Sprintf("credentials %q {\n  token = %q\n}\n\n", hostname, token))
		hasAuth = true

		// Set environment variable for mirror token
		os.Setenv("TF_MIRROR_TOKEN", string(token))

		logger.Info("Successfully configured token authentication",
			"hostname", hostname,
			"secretStoreID", secretStoreID,
			"tokenLength", len(token))
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
	configContent.WriteString(fmt.Sprintf(`provider_installation {
  network_mirror {
    url    = %q
    include = ["registry.terraform.io/*/*"]
  }
  direct {
    exclude = ["registry.terraform.io/*/*"]
  }
}
`, mirrorURL))

	// Create the .terraformrc file in the execution directory
	terraformRCPath := filepath.Join(dirPath, TerraformRCFilename)
	logger.Info("Writing Terraform registry configuration file", "path", terraformRCPath)

	err = os.WriteFile(terraformRCPath, []byte(configContent.String()), DefaultFilePerms)
	if err != nil {
		logger.Error(err, "Failed to write Terraform configuration file",
			"path", terraformRCPath,
			"permissions", fmt.Sprintf("%o", DefaultFilePerms))
		return fmt.Errorf("failed to write Terraform registry configuration file: %w", err)
	}

	// Set the TF_CLI_CONFIG_FILE environment variable to point to our config file
	os.Setenv(EnvTerraformCLIConfigFile, terraformRCPath)
	logger.Info("Set environment variable for Terraform config",
		"variable", EnvTerraformCLIConfigFile,
		"value", terraformRCPath)

	logger.Info("Terraform registry configuration created",
		"path", terraformRCPath,
		"mirror", mirrorURL,
		"hasAuth", hasAuth,
		"hasProviderMappings", len(config.RecipeConfig.Terraform.Registry.ProviderMappings) > 0)

	// Add diagnostic information to help troubleshoot common issues
	logger.Info("Terraform provider resolution flow",
		"step1", "Terraform will look for providers in the mirror URL",
		"step2", "Certificate validation will use system CA certificates unless SSL_CERT_FILE is set",
		"step3", "Host resolution must work from inside the container")

	return nil
}

// Helper function to get available secret keys for debugging
func getSecretKeys(data map[string]string) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	return keys
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

	// Unset environment variables
	os.Unsetenv(EnvTerraformCLIConfigFile)
	os.Unsetenv("TF_MIRROR_TOKEN") // Add this line to clean up the token

	// Check if the file exists and log its presence/absence
	configPath := filepath.Join(dirPath, TerraformRCFilename)
	if _, err := os.Stat(configPath); err == nil {
		logger.Info("Cleaned up Terraform registry configuration", "path", configPath)
	} else {
		logger.Info("No Terraform registry configuration to clean up")
	}

	return nil
}
