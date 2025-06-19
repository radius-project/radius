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
	"log"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
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
	// Default to latest version if not specified
	var tfVersion *version.Version
	if terraformConfig.Version != nil && terraformConfig.Version.Version != "" {
		v, err := version.NewVersion(terraformConfig.Version.Version)
		if err != nil {
			return "", fmt.Errorf("failed to parse terraform version: %w", err)
		}
		tfVersion = v
	}

	// Validate we have TLS configuration or authentication that requires custom source
	if terraformConfig.Version == nil {
		return "", fmt.Errorf("version configuration is required")
	}

	hasAuthentication := terraformConfig.Version.Authentication != nil
	hasTLSConfig := terraformConfig.Version.TLS != nil &&
		(terraformConfig.Version.TLS.SkipVerify || terraformConfig.Version.TLS.CACertificate != nil)

	if !hasAuthentication && !hasTLSConfig {
		return "", fmt.Errorf("this function should only be used when TLS configuration or authentication is required")
	}

	// Create custom source
	customSource := &CustomRegistrySource{
		Product:    product.Terraform,
		Version:    tfVersion,
		BaseURL:    getAPIBaseURL(terraformConfig),
		InstallDir: installDir,
	}

	log.Printf("Base URL for Terraform releases: %s", customSource.BaseURL)

	// Handle TLS configuration if present
	if terraformConfig.Version.TLS != nil {
		tlsConfig := terraformConfig.Version.TLS

		// Handle skip verification
		if tlsConfig.SkipVerify {
			customSource.InsecureSkipVerify = true
		}

		// Handle custom CA certificate
		if tlsConfig.CACertificate != nil {
			secretData, ok := secrets[tlsConfig.CACertificate.Source]
			if !ok {
				return "", fmt.Errorf("CA certificate secret store not found: %s", tlsConfig.CACertificate.Source)
			}

			caCertPEM, ok := secretData.Data[tlsConfig.CACertificate.Key]
			if !ok {
				return "", fmt.Errorf("CA certificate key '%s' not found in secret store", tlsConfig.CACertificate.Key)
			}

			customSource.CACertPEM = []byte(caCertPEM)
		}
	}

	// Handle authentication if configured
	if terraformConfig.Version.Authentication != nil {
		// Handle token authentication
		if terraformConfig.Version.Authentication.Token != nil {
			secretData, ok := secrets[terraformConfig.Version.Authentication.Token.Secret]
			if !ok {
				return "", fmt.Errorf("authentication secret store not found: %s", terraformConfig.Version.Authentication.Token.Secret)
			}

			token, ok := secretData.Data["token"]
			if !ok {
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

	// If no version specified, we need to fetch latest
	if customSource.Version == nil {
		// For custom source, we'll need to implement latest version lookup
		// For now, return an error
		return "", fmt.Errorf("latest version lookup not yet implemented for custom registry source")
	}

	// Install directly using the custom source
	return customSource.Install(ctx)
}

// getAPIBaseURL returns the API base URL from config or empty string for default
func getAPIBaseURL(terraformConfig datamodel.TerraformConfigProperties) string {
	if terraformConfig.Version != nil && terraformConfig.Version.ReleasesAPIBaseURL != "" {
		return terraformConfig.Version.ReleasesAPIBaseURL
	}
	return "https://releases.hashicorp.com"
}
