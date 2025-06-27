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
	EnvVars    map[string]string
	TempFiles  []string
}

// ConfigureTerraformRegistry sets up Terraform registry configuration for private registries.
// It creates a .terraformrc file with the registry mirror and sets up authentication via environment variables.
// Returns a RegistryConfig struct that tracks created resources for cleanup.
func ConfigureTerraformRegistry(
	ctx context.Context,
	config recipes.Configuration,
	secrets map[string]recipes.SecretData,
	dirPath string,
) (*RegistryConfig, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Initialize the registry config to track what we create
	regConfig := &RegistryConfig{
		EnvVars: make(map[string]string),
	}

	// Token will be extracted from secrets if authentication is configured
	var token []byte

	// Check if registry mirror configuration exists
	if config.RecipeConfig.Terraform.Registry == nil || config.RecipeConfig.Terraform.Registry.Mirror == "" {
		logger.Info("No Terraform registry mirror configured, skipping registry configuration")
		return nil, nil
	}

	logger.Info("Starting Terraform registry configuration",
		"mirror", config.RecipeConfig.Terraform.Registry.Mirror,
		"hasAuthentication", config.RecipeConfig.Terraform.Registry.Authentication.Token != nil,
		"hasTLS", config.RecipeConfig.Terraform.Registry.TLS != nil,
		"workingDir", dirPath)

	// Extract and validate the mirror URL
	mirrorURL := config.RecipeConfig.Terraform.Registry.Mirror
	logger.Info("Normalizing mirror URL", "originalURL", mirrorURL)

	// Check if URL is malformed first (e.g., starts with ://)
	if strings.HasPrefix(mirrorURL, "://") {
		logger.Error(nil, "Invalid mirror URL format", "url", mirrorURL)
		return nil, fmt.Errorf("invalid terraform registry mirror URL: %s", mirrorURL)
	}

	// Try parsing the URL as-is
	parsedURL, err := url.Parse(mirrorURL)
	if err != nil {
		logger.Error(err, "Failed to parse mirror URL", "url", mirrorURL)
		return nil, fmt.Errorf("invalid terraform registry mirror URL: %w", err)
	}

	// If no scheme is present, add https:// and reparse
	// This handles cases like "example.com" or "example.com:8443"
	if parsedURL.Scheme == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https" && strings.Contains(mirrorURL, ":")) {
		// For URLs without a scheme, Go's url.Parse may misinterpret the host as scheme
		// e.g., "example.com:8443" becomes scheme="example.com" instead of host="example.com:8443"
		// So we always add https:// for scheme-less URLs
		logger.Info("Adding https:// scheme to URL", "originalURL", mirrorURL)
		mirrorURL = "https://" + mirrorURL
		parsedURL, err = url.Parse(mirrorURL)
		if err != nil {
			logger.Error(err, "Failed to parse URL after adding scheme", "url", mirrorURL)
			return nil, fmt.Errorf("invalid terraform registry mirror URL: %w", err)
		}
	}

	// Use Host() instead of Hostname() to preserve port information
	host := parsedURL.Host
	if host == "" {
		logger.Error(nil, "Empty host in mirror URL", "originalURL", config.RecipeConfig.Terraform.Registry.Mirror, "parsedURL", mirrorURL)
		return nil, fmt.Errorf("empty host in mirror URL: %s", config.RecipeConfig.Terraform.Registry.Mirror)
	}

	logger.Info("Mirror URL normalized", "normalizedURL", parsedURL.String(), "host", host)

	// Begin building Terraform configuration
	var configContent strings.Builder

	// Handle authentication
	auth := config.RecipeConfig.Terraform.Registry.Authentication

	// Token authentication
	if auth.Token != nil && auth.Token.Secret != "" {
		logger.Info("Configuring token authentication",
			"secretStore", auth.Token.Secret,
			"additionalHostsCount", len(auth.AdditionalHosts))

		secretStoreID := auth.Token.Secret

		// Get token from secret store
		if secrets == nil {
			logger.Error(nil, "No secrets available for token authentication")
			return nil, fmt.Errorf("no secrets available for token authentication")
		}

		secretData, secretExists := secrets[secretStoreID]
		if !secretExists {
			availableSecrets := make([]string, 0, len(secrets))
			for k := range secrets {
				availableSecrets = append(availableSecrets, k)
			}
			logger.Error(nil, "Secret store not found",
				"secretStore", secretStoreID,
				"availableSecrets", availableSecrets)
			return nil, fmt.Errorf("secret store %q not found", secretStoreID)
		}

		logger.Info("Found secret data",
			"secretStore", secretStoreID,
			"dataKeys", len(secretData.Data))

		var tokenExists bool
		tokenString, tokenExists := secretData.Data["token"]
		if !tokenExists {
			availableKeys := make([]string, 0, len(secretData.Data))
			for k := range secretData.Data {
				availableKeys = append(availableKeys, k)
			}
			logger.Error(nil, "Token key not found in secret data",
				"secretStore", secretStoreID,
				"availableKeys", availableKeys)
			return nil, fmt.Errorf("token not found in secret store %q", secretStoreID)
		}

		// Convert string to []byte
		token = []byte(tokenString)

		// Log token info (safely)
		logger.Info("Successfully extracted token",
			"tokenLength", len(token),
			"tokenPrefix", getTokenPrefix(string(token)))

		// Use environment variables instead of credentials blocks in the config file.
		// This is necessary when using self-signed certificates, as Terraform requires
		// the TLS configuration to be set via environment variables for the initial
		// connection to download the provider index files.
		envVarName, envVarValue, err := getTerraformTokenEnv(host, string(token))
		if err != nil {
			logger.Error(err, "Failed to prepare token environment variable", "host", host)
			return nil, fmt.Errorf("failed to prepare token for %s: %w", host, err)
		}
		logger.Info("Setting environment variable for token authentication",
			"envVar", envVarName,
			"envValue", envVarValue,
			"host", host,
			"token", string(token))
		regConfig.EnvVars[envVarName] = envVarValue

		logger.Info("Configured token authentication",
			"host", host,
			"envVar", envVarName,
			"secretStoreID", secretStoreID)

		// Handle additional hosts if configured
		if len(auth.AdditionalHosts) > 0 {
			logger.Info("Configuring authentication for additional hosts", "hosts", auth.AdditionalHosts)

			// Apply same token to each additional host
			for _, additionalHost := range auth.AdditionalHosts {
				if additionalHost == "" || additionalHost == host {
					logger.Info("Skipping host", "host", additionalHost, "reason", "empty or duplicate")
					continue // Skip empty or duplicate hosts
				}

				// Get environment variable name and value
				envVarName, envVarValue, err := getTerraformTokenEnv(additionalHost, string(token))
				if err != nil {
					logger.Error(err, "Failed to prepare token for additional host", "host", additionalHost)
					return nil, fmt.Errorf("failed to prepare token for additional host %s: %w", additionalHost, err)
				}
				regConfig.EnvVars[envVarName] = envVarValue

				logger.Info("Added token authentication for additional host",
					"host", additionalHost,
					"envVar", envVarName)
			}
		}
	}

	// Log TLS configuration details
	if config.RecipeConfig.Terraform.Registry.TLS != nil {
		logger.Info("Registry TLS configuration found",
			"skipVerify", config.RecipeConfig.Terraform.Registry.TLS.SkipVerify,
			"hasCACert", config.RecipeConfig.Terraform.Registry.TLS.CACertificate != nil)

		if config.RecipeConfig.Terraform.Registry.TLS.SkipVerify {
			// Add TF_INSECURE_SKIP_TLS_VERIFY environment variable
			regConfig.EnvVars["TF_INSECURE_SKIP_TLS_VERIFY"] = "1"
			logger.Info("Added TF_INSECURE_SKIP_TLS_VERIFY environment variable for TLS skip")

			if parsedURL.Scheme == "https" {
				logger.Info("WARNING: TLS skipVerify is set for HTTPS registry. Using TF_INSECURE_SKIP_TLS_VERIFY to bypass certificate verification.")
			}
		}
	} else {
		logger.Info("No TLS configuration found for registry")
	}

	// Handle CA certificate if provided
	if config.RecipeConfig.Terraform.Registry.TLS != nil &&
		config.RecipeConfig.Terraform.Registry.TLS.CACertificate != nil &&
		config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Source != "" {

		logger.Info("Configuring CA certificate for registry",
			"secretStore", config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Source,
			"key", config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Key)

		// Get CA certificate from secrets
		secretStoreID := config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Source
		secretKey := config.RecipeConfig.Terraform.Registry.TLS.CACertificate.Key

		// Log available secrets for debugging
		if secrets != nil {
			availableSecrets := make([]string, 0, len(secrets))
			for k := range secrets {
				availableSecrets = append(availableSecrets, k)
			}
			logger.Info("Looking for CA certificate in secrets",
				"targetSecretStore", secretStoreID,
				"targetKey", secretKey,
				"availableSecrets", availableSecrets)
		}

		// Check if secrets map exists first
		if secrets == nil {
			logger.Info("No secrets available, skipping CA certificate configuration")
		} else if secretData, ok := secrets[secretStoreID]; ok {
			logger.Info("Found CA certificate secret store",
				"secretStore", secretStoreID,
				"dataKeysCount", len(secretData.Data))

			if caCert, ok := secretData.Data[secretKey]; ok {
				logger.Info("Successfully extracted CA certificate",
					"certLength", len(caCert),
					"certPreview", getCertPreview(string(caCert)))

				// Write CA certificate to file
				caCertPath := filepath.Join(dirPath, "terraform-registry-ca.pem")
				logger.Info("Writing CA certificate to file",
					"path", caCertPath,
					"size", len(caCert))

				if err := os.WriteFile(caCertPath, []byte(caCert), 0600); err != nil {
					logger.Error(err, "Failed to write CA certificate", "path", caCertPath)
					return nil, fmt.Errorf("failed to write CA certificate: %w", err)
				}

				// Verify file was written
				if stat, err := os.Stat(caCertPath); err == nil {
					logger.Info("CA certificate file written successfully",
						"path", caCertPath,
						"size", stat.Size(),
						"mode", stat.Mode())
				}

				regConfig.TempFiles = append(regConfig.TempFiles, caCertPath)

				// Store environment variables for CA certificate
				// These are used for HTTPS operations during provider downloads from the registry
				// Note: Git operations use GIT_SSL_CAINFO which is set separately in recipe TLS config
				regConfig.EnvVars["SSL_CERT_FILE"] = caCertPath
				regConfig.EnvVars["CURL_CA_BUNDLE"] = caCertPath

				logger.Info("CA certificate configured for registry operations",
					"path", caCertPath,
					"SSL_CERT_FILE", caCertPath,
					"CURL_CA_BUNDLE", caCertPath)
			} else {
				availableKeys := make([]string, 0, len(secretData.Data))
				for k := range secretData.Data {
					availableKeys = append(availableKeys, k)
				}
				logger.Error(nil, "CA certificate key not found in secret data",
					"secretStore", secretStoreID,
					"requestedKey", secretKey,
					"availableKeys", availableKeys)
				return nil, fmt.Errorf("CA certificate not found in secret store %q with key %q", secretStoreID, secretKey)
			}
		} else {
			logger.Error(nil, "Secret store not found for CA certificate",
				"requestedSecretStore", secretStoreID)
			return nil, fmt.Errorf("secret store %q not found for CA certificate", secretStoreID)
		}
	}

	// Add provider installation configuration
	logger.Info("Creating provider installation configuration",
		"mirrorURL", parsedURL.String(),
		"includePattern", "*/*/*",
		"excludePattern", "*/*/*")

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

	// Add credentials block for authentication (only if token is configured)
	if auth.Token != nil && auth.Token.Secret != "" && len(token) > 0 {
		logger.Info("Adding credentials block to .terraformrc",
			"host", host,
			"tokenLength", len(token))

		configContent.WriteString(fmt.Sprintf(`
credentials %q {
  token = %q
}
`, host, string(token)))
	}

	// Create the .terraformrc file in the execution directory
	terraformRCPath := filepath.Join(dirPath, TerraformRCFilename)
	logger.Info("Writing Terraform registry configuration file",
		"path", terraformRCPath,
		"contentLength", configContent.Len())

	err = os.WriteFile(terraformRCPath, []byte(configContent.String()), DefaultFilePerms)
	if err != nil {
		logger.Error(err, "Failed to write Terraform registry configuration file", "path", terraformRCPath)
		return nil, fmt.Errorf("failed to write Terraform registry configuration file: %w", err)
	}

	// Verify file was written
	if stat, err := os.Stat(terraformRCPath); err == nil {
		logger.Info("Terraform registry configuration file written successfully",
			"path", terraformRCPath,
			"size", stat.Size(),
			"mode", stat.Mode())

		// Log the content for debugging (be careful not to log sensitive tokens in production)
		if content, err := os.ReadFile(terraformRCPath); err == nil {
			// Mask the token in the log
			maskedContent := string(content)
			if auth.Token != nil && auth.Token.Secret != "" && len(token) > 0 {
				maskedContent = strings.ReplaceAll(maskedContent, string(token), "***MASKED***")
			}
			logger.Info("Terraform configuration content",
				"content", maskedContent)
		}
	}

	regConfig.ConfigPath = terraformRCPath

	// Store the TF_CLI_CONFIG_FILE environment variable
	regConfig.EnvVars[EnvTerraformCLIConfigFile] = terraformRCPath

	// Log all environment variables that will be set
	envVarKeys := make([]string, 0, len(regConfig.EnvVars))
	for k, v := range regConfig.EnvVars {
		envVarKeys = append(envVarKeys, k)
		// Log specific important env vars
		if k == "TF_INSECURE_SKIP_TLS_VERIFY" {
			logger.Info("TLS verification will be skipped", "envVar", k, "value", v)
		}
	}

	// Log if credentials were added
	hasCredentials := auth.Token != nil && auth.Token.Secret != "" && len(token) > 0

	logger.Info("Terraform registry configuration complete",
		"configPath", terraformRCPath,
		"mirrorURL", parsedURL.String(),
		"hasCredentialsBlock", hasCredentials,
		"envVarCount", len(regConfig.EnvVars),
		"envVars", envVarKeys,
		"tempFilesCount", len(regConfig.TempFiles))

	return regConfig, nil
}

// getTerraformTokenEnv prepares the TF_TOKEN_* environment variable for a hostname
// Returns the environment variable name and value
func getTerraformTokenEnv(hostname string, token string) (string, string, error) {
	// Replace dots and colons with underscores in hostname (for ports)
	envHostname := strings.ReplaceAll(hostname, ".", "_")
	envHostname = strings.ReplaceAll(envHostname, ":", "_")
	envVarName := fmt.Sprintf("TF_TOKEN_%s", envHostname)
	return envVarName, token, nil
}

// CleanupTerraformRegistryConfig removes the Terraform registry configuration and unsets environment variables
func CleanupTerraformRegistryConfig(ctx context.Context, config *RegistryConfig) error {
	if config == nil {
		return nil
	}

	logger := ucplog.FromContextOrDiscard(ctx)

	// Note: We no longer unset environment variables since they are now passed
	// to the Terraform process rather than set on the current process.
	// The cleanup is handled by the tfexec library when the process terminates.

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

// Helper functions for safe logging

// getTokenPrefix returns a safe preview of a token (first 8 chars only)
func getTokenPrefix(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:8] + "..."
}

// getCertPreview returns a safe preview of certificate content
func getCertPreview(cert string) string {
	lines := strings.Split(cert, "\n")
	if len(lines) > 0 {
		return lines[0] // Usually "-----BEGIN CERTIFICATE-----"
	}
	return "empty"
}
