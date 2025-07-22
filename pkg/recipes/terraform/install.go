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
	"time"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/hc-install/src"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/components/metrics"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/customsource"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
)

const (
	installSubDir                     = "install"
	installVerificationRetryCount     = 5
	installVerificationRetryDelaySecs = 3
)

// validateReleasesURL validates the releases API URL and ensures it uses HTTPS unless explicitly allowed
func validateReleasesURL(ctx context.Context, releasesURL string, tlsConfig *datamodel.TLSConfig) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if releasesURL == "" {
		logger.Info("No custom releases URL provided, using default HashiCorp releases site")
		return nil // Default HashiCorp releases site will be used
	}

	logger.Info("Validating releases API URL", "url", releasesURL)
	parsedURL, err := url.Parse(releasesURL)
	if err != nil {
		logger.Error(err, "Failed to parse releases API URL", "url", releasesURL)
		return fmt.Errorf("invalid releases API URL: %w", err)
	}

	// Check if URL uses HTTP instead of HTTPS
	if parsedURL.Scheme == "http" {
		if tlsConfig != nil && tlsConfig.SkipVerify {
			// Allow HTTP only if skipVerify is explicitly set to true
			logger.Info("Allowing HTTP URL due to TLS skip verify setting", "url", releasesURL)
			return nil
		}
		logger.Error(nil, "Releases API URL must use HTTPS for security", "url", releasesURL)
		return fmt.Errorf("releases API URL must use HTTPS for security. Use 'tls.skipVerify: true' to allow insecure connections (not recommended)")
	}

	if parsedURL.Scheme != "https" {
		logger.Error(nil, "Invalid URL scheme for releases API", "scheme", parsedURL.Scheme, "url", releasesURL)
		return fmt.Errorf("releases API URL must use either HTTP or HTTPS scheme, got: %s", parsedURL.Scheme)
	}

	logger.Info("Releases API URL validation passed", "url", releasesURL)
	return nil
}

// validateArchiveURL validates the archive URL and ensures it uses HTTPS unless explicitly allowed
func validateArchiveURL(ctx context.Context, archiveURL string, tlsConfig *datamodel.TLSConfig) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if archiveURL == "" {
		return nil
	}

	logger.Info("Validating archive URL", "url", archiveURL)
	parsedURL, err := url.Parse(archiveURL)
	if err != nil {
		logger.Error(err, "Failed to parse archive URL", "url", archiveURL)
		return fmt.Errorf("invalid archive URL: %w", err)
	}

	// Check if URL uses HTTP instead of HTTPS
	if parsedURL.Scheme == "http" {
		if tlsConfig != nil && tlsConfig.SkipVerify {
			// Allow HTTP only if skipVerify is explicitly set to true
			logger.Info("Allowing HTTP archive URL due to TLS skip verify setting", "url", archiveURL)
			return nil
		}
		logger.Error(nil, "Archive URL must use HTTPS for security", "url", archiveURL)
		return fmt.Errorf("archive URL must use HTTPS for security. Use 'tls.skipVerify: true' to allow insecure connections (not recommended)")
	}

	if parsedURL.Scheme != "https" {
		logger.Error(nil, "Invalid URL scheme for archive", "scheme", parsedURL.Scheme, "url", archiveURL)
		return fmt.Errorf("archive URL must use either HTTP or HTTPS scheme, got: %s", parsedURL.Scheme)
	}

	// Validate that the URL ends with .zip
	if filepath.Ext(parsedURL.Path) != ".zip" {
		logger.Error(nil, "Archive URL must point to a .zip file", "extension", filepath.Ext(parsedURL.Path), "url", archiveURL)
		return fmt.Errorf("archive URL must point to a .zip file, got: %s", filepath.Ext(parsedURL.Path))
	}

	logger.Info("Archive URL validation passed", "url", archiveURL)
	return nil
}

// Install installs Terraform under /install in the provided Terraform root directory for the resource.
// It supports multiple installation methods:
// 1. Pre-mounted binaries from containers (for air-gapped environments)
// 2. Pre-downloaded binaries from a standard location
// 3. Download from HashiCorp releases (with custom URLs and TLS configuration)
// It also configures Terraform logging based on the provided logLevel.
// Returns the path to the installed Terraform executable or an error if installation fails.
func Install(ctx context.Context, installer *install.Installer, tfDir string, terraformConfig datamodel.TerraformConfigProperties, secrets map[string]recipes.SecretData, logLevel string) (*tfexec.Terraform, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Check if Terraform is pre-mounted from a container (e.g., via init container)
	preMountedPath := filepath.Join(tfDir, "terraform")
	if _, err := os.Stat(preMountedPath); err == nil {
		logger.Info(fmt.Sprintf("Found pre-mounted Terraform binary at: %q", preMountedPath))

		// Create a new instance of tfexec.Terraform with the pre-mounted binary
		tf, err := NewTerraform(ctx, tfDir, preMountedPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create Terraform instance with pre-mounted binary: %w", err)
		}

		// Verify the pre-mounted Terraform binary is working
		_, _, err = tf.Version(ctx, false)
		if err != nil {
			logger.Info(fmt.Sprintf("Pre-mounted Terraform binary is not working properly: %s. Falling back to download.", err.Error()))
		} else {
			logger.Info("Successfully using pre-mounted Terraform binary")
			// Configure Terraform logs for the pre-mounted binary
			configureTerraformLogs(ctx, tf, logLevel)
			return tf, nil
		}
	}

	// Create Terraform installation directory
	installDir := filepath.Join(tfDir, installSubDir)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for terraform installation for resource: %w", err)
	}

	logger.Info(fmt.Sprintf("Installing Terraform in the directory: %q", installDir))

	// Check if pre-downloaded Terraform binary exists and copy it to installDir
	preMountedBinaryPath := "/terraform/terraform"
	markerFile := "/terraform/.terraform-source"

	if _, err := os.Stat(preMountedBinaryPath); err == nil {
		if _, err := os.Stat(markerFile); err == nil {
			logger.Info("Copying pre-downloaded Terraform binary to install directory")
			if data, err := os.ReadFile(preMountedBinaryPath); err == nil {
				if err := os.WriteFile(filepath.Join(installDir, "terraform"), data, 0755); err == nil {
					logger.Info("Successfully copied pre-downloaded Terraform binary")

					// Create a new instance of tfexec.Terraform with the copied binary
					tf, err := NewTerraform(ctx, tfDir, filepath.Join(installDir, "terraform"))
					if err != nil {
						return nil, fmt.Errorf("failed to create Terraform instance with copied binary: %w", err)
					}

					// Verify the copied Terraform binary is working
					_, _, err = tf.Version(ctx, false)
					if err != nil {
						logger.Info(fmt.Sprintf("Copied Terraform binary is not working properly: %s. Falling back to download.", err.Error()))
					} else {
						logger.Info("Successfully using copied pre-downloaded Terraform binary")
						// Configure Terraform logs for the copied binary
						configureTerraformLogs(ctx, tf, logLevel)
						return tf, nil
					}
				}
			}
		}
	}

	// Validate the URLs
	var tlsConfig *datamodel.TLSConfig
	var useArchiveURL bool
	if terraformConfig.Version != nil {
		tlsConfig = terraformConfig.Version.TLS

		// Check if releasesArchiveUrl is provided (takes precedence)
		if terraformConfig.Version.ReleasesArchiveURL != "" {
			if err := validateArchiveURL(ctx, terraformConfig.Version.ReleasesArchiveURL, tlsConfig); err != nil {
				return nil, err
			}
			useArchiveURL = true
			logger.Info(fmt.Sprintf("Using direct archive URL: %s", terraformConfig.Version.ReleasesArchiveURL))
		} else if err := validateReleasesURL(ctx, terraformConfig.Version.ReleasesAPIBaseURL, tlsConfig); err != nil {
			return nil, err
		}
	}

	// Check if we need to use custom source for TLS configuration, authentication, or archive URL
	needsCustomSource := useArchiveURL ||
		(tlsConfig != nil && (tlsConfig.SkipVerify || tlsConfig.CACertificate != nil)) ||
		(terraformConfig.Version != nil && terraformConfig.Version.Authentication != nil)

	installStartTime := time.Now()

	var execPath string
	var err error

	if needsCustomSource {
		logger.Info("Using custom source for Terraform installation due to TLS configuration or authentication")

		// Log security warnings if applicable
		if tlsConfig != nil && tlsConfig.SkipVerify {
			logger.Info("WARNING: TLS verification is disabled for Terraform releases. This is insecure and should not be used in production.")
		}
		if tlsConfig != nil && tlsConfig.CACertificate != nil {
			logger.Info("Using custom CA certificate for Terraform releases")
		}
		if terraformConfig.Version != nil && terraformConfig.Version.Authentication != nil {
			logger.Info("Using authentication for Terraform releases")
		}
		if useArchiveURL {
			logger.Info("Using direct archive URL for Terraform download")
		}

		// Use custom installation method
		execPath, err = customsource.InstallTerraformWithTLS(ctx, installDir, terraformConfig, secrets)
		if err != nil {
			metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
				[]attribute.KeyValue{
					metrics.TerraformVersionAttrKey.String(terraformConfig.Version.Version),
					metrics.OperationStateAttrKey.String(metrics.FailedOperationState),
				},
			)
			return nil, fmt.Errorf("failed to install terraform with custom TLS: %w", err)
		}
	} else {
		// Use standard hc-install sources
		var terraformSource src.Source

		if terraformConfig.Version != nil && terraformConfig.Version.Version != "" {
			logger.Info(fmt.Sprintf("Installing Terraform version: %s", terraformConfig.Version.Version))

			version, err := version.NewVersion(terraformConfig.Version.Version)
			if err != nil {
				return nil, fmt.Errorf("failed to parse terraform version: %w", err)
			}

			if terraformConfig.Version.ReleasesAPIBaseURL != "" {
				logger.Info(fmt.Sprintf("Using custom releases API base URL: %s", terraformConfig.Version.ReleasesAPIBaseURL))

				terraformSource = &releases.ExactVersion{
					Product:    product.Terraform,
					InstallDir: installDir,
					Version:    version,
					ApiBaseURL: terraformConfig.Version.ReleasesAPIBaseURL,
				}
			} else {
				logger.Info("Using default releases API base URL")

				terraformSource = &releases.ExactVersion{
					Product:    product.Terraform,
					InstallDir: installDir,
					Version:    version,
				}
			}
		} else {
			logger.Info("Installing latest version of Terraform")

			if terraformConfig.Version != nil && terraformConfig.Version.ReleasesAPIBaseURL != "" {
				logger.Info(fmt.Sprintf("Using custom releases API base URL: %s", terraformConfig.Version.ReleasesAPIBaseURL))

				terraformSource = &releases.LatestVersion{
					Product:    product.Terraform,
					InstallDir: installDir,
					ApiBaseURL: terraformConfig.Version.ReleasesAPIBaseURL,
				}
			} else {
				logger.Info("Using default releases API base URL")

				terraformSource = &releases.LatestVersion{
					Product:    product.Terraform,
					InstallDir: installDir,
				}
			}
		}

		// Re-visit this: consider checking if an existing installation of same version of Terraform is available.
		// For initial iteration we will always install Terraform for every execution of the recipe driver.
		execPath, err = installer.Ensure(ctx, []src.Source{terraformSource})
		if err != nil {
			// Determine version for metrics
			versionStr := "latest"
			if terraformConfig.Version != nil && terraformConfig.Version.Version != "" {
				versionStr = terraformConfig.Version.Version
			}

			metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
				[]attribute.KeyValue{
					metrics.TerraformVersionAttrKey.String(versionStr),
					metrics.OperationStateAttrKey.String(metrics.FailedOperationState),
				},
			)
			return nil, err
		}
	}

	// Determine the version string for logging and metrics
	versionStr := "latest"
	if terraformConfig.Version != nil && terraformConfig.Version.Version != "" {
		versionStr = terraformConfig.Version.Version
	}

	metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
		[]attribute.KeyValue{
			metrics.TerraformVersionAttrKey.String(versionStr),
			metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState),
		},
	)

	if versionStr == "latest" {
		logger.Info(fmt.Sprintf("Terraform latest version installed to: %q", execPath))
	} else {
		logger.Info(fmt.Sprintf("Terraform version %s installed to: %q", versionStr, execPath))
	}

	// Create a new instance of tfexec.Terraform with current Terraform installation path
	tf, err := NewTerraform(ctx, tfDir, execPath)
	if err != nil {
		return nil, err
	}

	// Verify Terraform installation is complete before proceeding
	for attempt := 0; attempt <= installVerificationRetryCount; attempt++ {
		_, _, err = tf.Version(ctx, false)
		if err == nil {
			metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallVerificationDuration(ctx, installStartTime,
				[]attribute.KeyValue{
					metrics.TerraformVersionAttrKey.String("latest"),
					metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState),
				},
			)
			break
		}
		if attempt < installVerificationRetryCount {
			logger.Info(fmt.Sprintf("Failed to verify Terraform installation completion: %s. Retrying after %d seconds", err.Error(), installVerificationRetryDelaySecs))
			metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallVerificationDuration(ctx, installStartTime,
				[]attribute.KeyValue{
					metrics.TerraformVersionAttrKey.String("latest"),
					metrics.OperationStateAttrKey.String(metrics.FailedOperationState),
				},
			)
			time.Sleep(time.Duration(installVerificationRetryDelaySecs) * time.Second)
			continue
		}
		return nil, fmt.Errorf("failed to verify Terraform installation completion after %d attempts. Error: %s", installVerificationRetryCount, err.Error())
	}

	// Configure Terraform logs once Terraform installation is complete
	configureTerraformLogs(ctx, tf, logLevel)

	return tf, nil
}
