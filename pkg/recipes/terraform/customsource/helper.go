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

package customsource

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

// InstallTerraformWithTLS installs Terraform using the custom source when TLS
// configuration is required. This is an alternative to using hc-install when
// custom CA certificates or skip verification is needed.
func InstallTerraformWithTLS(
	ctx context.Context,
	installDir string,
	terraformConfig datamodel.TerraformConfigProperties,
	secrets map[string]recipes.SecretData,
) (string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Default to latest version if not specified
	var tfVersion *version.Version
	if terraformConfig.Version != nil && terraformConfig.Version.Version != "" {
		v, err := version.NewVersion(terraformConfig.Version.Version)
		if err != nil {
			return "", fmt.Errorf("failed to parse terraform version: %w", err)
		}
		tfVersion = v
	}

	// Validate we have TLS configuration, authentication, or archive URL that requires custom source
	if terraformConfig.Version == nil {
		return "", fmt.Errorf("version configuration is required")
	}

	hasAuthentication := terraformConfig.Version.Authentication != nil
	hasTLSConfig := terraformConfig.Version.TLS != nil &&
		(terraformConfig.Version.TLS.SkipVerify || terraformConfig.Version.TLS.CACertificate != nil)
	hasArchiveURL := terraformConfig.Version.ReleasesArchiveURL != ""

	if !hasAuthentication && !hasTLSConfig && !hasArchiveURL {
		return "", fmt.Errorf("this function should only be used when TLS configuration, authentication, or archive URL is required")
	}

	// Create custom source
	customSource := &CustomRegistrySource{
		Product:    product.Terraform,
		Version:    tfVersion,
		BaseURL:    getAPIBaseURL(terraformConfig),
		InstallDir: installDir,
	}

	// If archive URL is provided, use it directly
	if hasArchiveURL {
		customSource.ArchiveURL = terraformConfig.Version.ReleasesArchiveURL
		logger.Info("Using direct archive URL for Terraform download", "archiveURL", customSource.ArchiveURL)
	} else {
		logger.Info("Using releases API for Terraform download", "baseURL", customSource.BaseURL)
	}

	// Handle TLS configuration if present
	if terraformConfig.Version.TLS != nil {
		tlsConfig := terraformConfig.Version.TLS

		// Handle skip verification
		if tlsConfig.SkipVerify {
			customSource.InsecureSkipVerify = true
			logger.Info("TLS verification disabled for Terraform download", "skipVerify", true)
		}

		// Handle custom CA certificate
		if tlsConfig.CACertificate != nil {
			logger.Info("Configuring custom CA certificate", "secretSource", tlsConfig.CACertificate.Source)
			secretData, ok := secrets[tlsConfig.CACertificate.Source]
			if !ok {
				logger.Error(nil, "CA certificate secret store not found", "secretSource", tlsConfig.CACertificate.Source)
				return "", fmt.Errorf("CA certificate secret store not found: %s", tlsConfig.CACertificate.Source)
			}

			caCertPEM, ok := secretData.Data[tlsConfig.CACertificate.Key]
			if !ok {
				logger.Error(nil, "CA certificate key not found in secret store", "key", tlsConfig.CACertificate.Key)
				return "", fmt.Errorf("CA certificate key '%s' not found in secret store", tlsConfig.CACertificate.Key)
			}

			customSource.CACertPEM = []byte(caCertPEM)
		}
	}

	// Handle authentication if configured
	if terraformConfig.Version.Authentication != nil {
		// Handle token authentication
		if terraformConfig.Version.Authentication.Token != nil {
			logger.Info("Configuring token authentication", "secretSource", terraformConfig.Version.Authentication.Token.Secret)
			secretData, ok := secrets[terraformConfig.Version.Authentication.Token.Secret]
			if !ok {
				logger.Error(nil, "Authentication secret store not found", "secretSource", terraformConfig.Version.Authentication.Token.Secret)
				return "", fmt.Errorf("authentication secret store not found: %s", terraformConfig.Version.Authentication.Token.Secret)
			}

			token, ok := secretData.Data["token"]
			if !ok {
				logger.Error(nil, "Token not found in secret store", "secretSource", terraformConfig.Version.Authentication.Token.Secret)
				return "", fmt.Errorf("token not found in secret store")
			}

			// For token auth, we set the auth token directly (without "Bearer" prefix)
			// The custom source will add the appropriate header
			customSource.AuthToken = string(token)
		}
	}

	// Handle client certificate authentication in TLS config
	// This is handled at the HTTP client level, not as a separate auth token
	// The custom source implementation already creates a custom HTTP client with TLS config

	// If no version specified and not using archive URL, we need to fetch latest
	if customSource.Version == nil && !hasArchiveURL {
		// For custom source, we'll need to implement latest version lookup
		// For now, return an error
		logger.Error(nil, "Latest version lookup not implemented for custom registry source")
		return "", fmt.Errorf("latest version lookup not yet implemented for custom registry source")
	}

	// Install directly using the custom source
	logger.Info("Starting Terraform installation with custom source", "version", tfVersion, "hasAuth", hasAuthentication, "hasTLS", hasTLSConfig)
	return customSource.Install(ctx)
}

// getAPIBaseURL returns the API base URL from config or empty string for default
func getAPIBaseURL(terraformConfig datamodel.TerraformConfigProperties) string {
	if terraformConfig.Version != nil && terraformConfig.Version.ReleasesAPIBaseURL != "" {
		return terraformConfig.Version.ReleasesAPIBaseURL
	}
	// If we have an archive URL but no API base URL, we don't need the API
	if terraformConfig.Version != nil && terraformConfig.Version.ReleasesArchiveURL != "" {
		return ""
	}
	return "https://releases.hashicorp.com"
}
