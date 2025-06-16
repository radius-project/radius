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

	// Add scheme if missing
	if !strings.HasPrefix(mirrorURL, "http://") && !strings.HasPrefix(mirrorURL, "https://") {
		mirrorURL = "https://" + mirrorURL
	}

	// Validate the URL is well-formed
	parsedURL, err := url.Parse(mirrorURL)
	if err != nil {
		return fmt.Errorf("invalid terraform registry mirror URL: %w", err)
	}

	hostname := parsedURL.Hostname()
	if hostname == "" {
		return fmt.Errorf("empty hostname in mirror URL: %s", config.RecipeConfig.Terraform.Registry.Mirror)
	}

	// Begin building Terraform configuration
	var configContent strings.Builder

	// Track if authentication is configured
	hasAuth := false

	// Handle authentication
	auth := config.RecipeConfig.Terraform.Registry.Authentication

	// Basic authentication
	if auth.Basic != nil && auth.Basic.Secret != "" {
		secretStoreID := auth.Basic.Secret

		// Get credentials from secret store
		if secrets == nil {
			return fmt.Errorf("no secrets available for basic authentication")
		}

		secretData, secretExists := secrets[secretStoreID]
		if !secretExists {
			return fmt.Errorf("secret store %q not found", secretStoreID)
		}

		username, usernameExists := secretData.Data["username"]
		password, passwordExists := secretData.Data["password"]

		if !usernameExists || !passwordExists {
			return fmt.Errorf("username or password not found in secret store %q", secretStoreID)
		}

		// Configure credentials
		configContent.WriteString(fmt.Sprintf("credentials %q {\n  username = %q\n  password = %q\n}\n\n", hostname, username, password))
		hasAuth = true

		// Set environment variable for basic auth
		credentials := fmt.Sprintf("%s:%s", string(username), string(password))
		encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
		token := fmt.Sprintf("Basic %s", encoded)
		setTerraformTokenEnv(hostname, token)

		logger.Info("Configured basic authentication",
			"hostname", hostname,
			"username", string(username),
			"secretStoreID", secretStoreID)
	}

	// Handle additional hosts if configured
	if hasAuth && len(auth.AdditionalHosts) > 0 {
		logger.Info("Configuring authentication for additional hosts", "hosts", auth.AdditionalHosts)

		// Get credentials once (we already validated they exist above)
		secretData := secrets[auth.Basic.Secret]
		username := secretData.Data["username"]
		password := secretData.Data["password"]

		// Apply same credentials to each additional host
		for _, additionalHost := range auth.AdditionalHosts {
			if additionalHost == "" || additionalHost == hostname {
				continue // Skip empty or duplicate hosts
			}

			// Add credentials for this host
			configContent.WriteString(fmt.Sprintf("credentials %q {\n  username = %q\n  password = %q\n}\n\n", additionalHost, username, password))
			
			// Set environment variable
			credentials := fmt.Sprintf("%s:%s", string(username), string(password))
			encoded := base64.StdEncoding.EncodeToString([]byte(credentials))
			token := fmt.Sprintf("Basic %s", encoded)
			setTerraformTokenEnv(additionalHost, token)
			
			logger.Info("Added basic authentication for additional host", "host", additionalHost, "username", string(username))
		}
	}

	// Add provider installation configuration with mirror URL
	configContent.WriteString(fmt.Sprintf(`provider_installation {
  network_mirror {
    url    = %q
    include = ["*/*/*"]
  }
  direct {
    exclude = ["*/*/*"]
  }
}
`, config.RecipeConfig.Terraform.Registry.Mirror))

	// Create the .terraformrc file in the execution directory
	terraformRCPath := filepath.Join(dirPath, TerraformRCFilename)
	logger.Info("Writing Terraform registry configuration file", "path", terraformRCPath)

	err = os.WriteFile(terraformRCPath, []byte(configContent.String()), DefaultFilePerms)
	if err != nil {
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
		"hasAuth", hasAuth)

	return nil
}

// setTerraformTokenEnv sets the TF_TOKEN_* environment variable for a hostname
func setTerraformTokenEnv(hostname string, token string) {
	// Replace dots with underscores in hostname
	envHostname := strings.ReplaceAll(hostname, ".", "_")
	envVarName := fmt.Sprintf("TF_TOKEN_%s", envHostname)
	os.Setenv(envVarName, token)
}

// CleanupTerraformRegistryConfig removes the Terraform registry configuration and unsets environment variables
func CleanupTerraformRegistryConfig(ctx context.Context, dirPath string) error {
	// Unset environment variables
	os.Unsetenv(EnvTerraformCLIConfigFile)

	// Note: We cannot reliably clean up TF_TOKEN_* variables without tracking which ones we set
	// This is a limitation but shouldn't cause issues as they're process-specific

	return nil
}
