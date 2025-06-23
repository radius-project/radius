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
	TempFiles  []string // Track temporary certificate files for cleanup
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
		envVar, err := setTerraformTokenEnv(host, string(token))
		if err != nil {
			return nil, fmt.Errorf("failed to set token for %s: %w", host, err)
		}
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
				envVar, err := setTerraformTokenEnv(additionalHost, string(token))
				if err != nil {
					return nil, fmt.Errorf("failed to set token for additional host %s: %w", additionalHost, err)
				}
				regConfig.EnvVars = append(regConfig.EnvVars, envVar)

				logger.Info("Added token authentication for additional host", "host", additionalHost)
			}
		}
	}

	// Log TLS configuration details
	if config.RecipeConfig.Terraform.Registry.TLS != nil {
		logger.Info("Registry TLS configuration found",
			"skipVerify", config.RecipeConfig.Terraform.Registry.TLS.SkipVerify,
			"hasCACert", config.RecipeConfig.Terraform.Registry.TLS.CACertificate != nil,
			"hasClientCert", config.RecipeConfig.Terraform.Registry.TLS.ClientCertificate != nil)

		if config.RecipeConfig.Terraform.Registry.TLS.SkipVerify && parsedURL.Scheme == "https" {
			logger.Info("WARNING: TLS skipVerify is set but Terraform does not support skipping TLS verification for provider mirrors. " +
				"You must either use HTTP, add the CA certificate to the system trust store, or use a filesystem mirror.")
		}
	} else {
		logger.Info("No TLS configuration found for registry")
	}

	// Handle CA certificate if provided
	if config.RecipeConfig.Terraform.Registry.TLS != nil &&
		config.RecipeConfig.Terraform.Registry.TLS.CACertificate != nil &&
		config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Source != "" {

		logger.Info("Configuring CA certificate for registry")

		// Get CA certificate from secrets
		secretStoreID := config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Source
		secretKey := config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Key

		// Log available secrets for debugging
		logger.Info("Looking for CA certificate", "secretStoreID", secretStoreID, "key", secretKey)
		if secrets != nil {
			var availableSecrets []string
			for k := range secrets {
				availableSecrets = append(availableSecrets, k)
			}
			logger.Info("Available secrets", "secrets", availableSecrets)
		}

		// Check if secrets map exists first
		if secrets == nil {
			logger.Info("No secrets available, skipping CA certificate configuration")
		} else if secretData, ok := secrets[secretStoreID]; ok {
			if caCert, ok := secretData.Data[secretKey]; ok {
				// Write CA certificate to file
				caCertPath := filepath.Join(dirPath, "terraform-registry-ca.pem")
				if err := os.WriteFile(caCertPath, []byte(caCert), 0600); err != nil {
					return nil, fmt.Errorf("failed to write CA certificate: %w", err)
				}

				regConfig.TempFiles = append(regConfig.TempFiles, caCertPath)

				// Set environment variables for CA certificate
				if err := os.Setenv("SSL_CERT_FILE", caCertPath); err != nil {
					return nil, fmt.Errorf("failed to set SSL_CERT_FILE: %w", err)
				}
				regConfig.EnvVars = append(regConfig.EnvVars, "SSL_CERT_FILE")

				if err := os.Setenv("CURL_CA_BUNDLE", caCertPath); err != nil {
					return nil, fmt.Errorf("failed to set CURL_CA_BUNDLE: %w", err)
				}
				regConfig.EnvVars = append(regConfig.EnvVars, "CURL_CA_BUNDLE")

				logger.Info("CA certificate configured", "path", caCertPath)
			} else {
				return nil, fmt.Errorf("CA certificate not found in secret store %q with key %q", secretStoreID, secretKey)
			}
		} else {
			return nil, fmt.Errorf("secret store %q not found for CA certificate", secretStoreID)
		}
	}

	// Handle client certificate if provided (for mTLS)
	if config.RecipeConfig.Terraform.Registry.TLS != nil &&
		config.RecipeConfig.Terraform.Registry.TLS.ClientCertificate != nil &&
		config.RecipeConfig.Terraform.Registry.TLS.ClientCertificate.Secret != "" {

		logger.Info("Configuring client certificate for registry mTLS")

		// Get client certificate and key from secrets
		secretStoreID := config.RecipeConfig.Terraform.Registry.TLS.ClientCertificate.Secret

		if secretData, ok := secrets[secretStoreID]; ok {
			// Write client certificate
			if clientCert, ok := secretData.Data["certificate"]; ok {
				clientCertPath := filepath.Join(dirPath, "terraform-registry-client.pem")
				if err := os.WriteFile(clientCertPath, []byte(clientCert), 0600); err != nil {
					return nil, fmt.Errorf("failed to write client certificate: %w", err)
				}
				regConfig.TempFiles = append(regConfig.TempFiles, clientCertPath)

				// Note: Terraform doesn't directly support client certificates via environment variables
				// This would require proxy with mTLS support or system-level configuration
				logger.Info("Client certificate written", "path", clientCertPath,
					"note", "Client certificates may require additional configuration")
			}

			// Write client key
			if clientKey, ok := secretData.Data["key"]; ok {
				clientKeyPath := filepath.Join(dirPath, "terraform-registry-client-key.pem")
				if err := os.WriteFile(clientKeyPath, []byte(clientKey), 0600); err != nil {
					return nil, fmt.Errorf("failed to write client key: %w", err)
				}
				regConfig.TempFiles = append(regConfig.TempFiles, clientKeyPath)

				logger.Info("Client key written", "path", clientKeyPath)
			}
		} else {
			return nil, fmt.Errorf("secret store %q not found for client certificate", secretStoreID)
		}
	}

	// Add provider installation configuration
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
	if err := os.Setenv(EnvTerraformCLIConfigFile, terraformRCPath); err != nil {
		return nil, fmt.Errorf("failed to set %s environment variable: %w", EnvTerraformCLIConfigFile, err)
	}
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
func setTerraformTokenEnv(hostname string, token string) (string, error) {
	// Replace dots and colons with underscores in hostname (for ports)
	envHostname := strings.ReplaceAll(hostname, ".", "_")
	envHostname = strings.ReplaceAll(envHostname, ":", "_")
	envVarName := fmt.Sprintf("TF_TOKEN_%s", envHostname)
	if err := os.Setenv(envVarName, token); err != nil {
		return "", fmt.Errorf("failed to set environment variable %s: %w", envVarName, err)
	}
	return envVarName, nil
}

// CleanupTerraformRegistryConfig removes the Terraform registry configuration and unsets environment variables
func CleanupTerraformRegistryConfig(ctx context.Context, config *RegistryConfig) error {
	if config == nil {
		return nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)

	// Unset all tracked environment variables
	for _, envVar := range config.EnvVars {
		if err := os.Unsetenv(envVar); err != nil {
			// Log the error but continue cleanup
			logger.Error(err, "Failed to unset environment variable", "variable", envVar)
		} else {
			logger.Info("Unset environment variable", "variable", envVar)
		}
	}

	// Remove the config file if it exists
	if config.ConfigPath != "" {
		if err := os.Remove(config.ConfigPath); err != nil && !os.IsNotExist(err) {
			logger.Info("Failed to remove Terraform config file", "path", config.ConfigPath, "error", err)
			// Don't return error as this is just cleanup
		}
	}

	// Remove temporary certificate files
	for _, tempFile := range config.TempFiles {
		if err := os.Remove(tempFile); err != nil && !os.IsNotExist(err) {
			logger.Info("Failed to remove temporary file", "path", tempFile, "error", err)
			// Don't return error as this is just cleanup
		} else {
			logger.Info("Removed temporary file", "path", tempFile)
		}
	}

	return nil
}
