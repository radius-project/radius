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
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-logr/logr"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/hc-install/src"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/components/metrics"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
)

const (
	installSubDir                     = "install"
	installVerificationRetryCount     = 5
	installVerificationRetryDelaySecs = 3

	// Default Terraform root path used when no configured root is provided.
	defaultTerraformRoot = "/terraform"

	// Global shared terraform binary paths (persistent hidden directory under terraform root)
	// Using .terraform-global as a more recognizable and persistent directory name
	globalTerraformDirName    = ".terraform-global"
	globalTerraformBinaryName = "terraform"
	globalTerraformMarkerName = ".terraform-ready"

	// Installer API paths - these are used by the `rad terraform install` command
	// and the Terraform installer REST API to pre-install specific versions.
	// The "current" symlink points to the active version's binary.
	installerCurrentSymlinkName = "current"
)

// InstallOptions configures how Terraform is installed and initialized.
type InstallOptions struct {
	// RootDir is the directory used to create the Terraform working directory for the caller.
	RootDir string

	// TerraformPath is the root directory where the Terraform installer writes binaries.
	// This should match the configured terraform.path value when set.
	TerraformPath string

	// LogLevel controls the verbosity of Terraform execution logs.
	LogLevel string
}

// terraformRootPath returns the Terraform root path, falling back to the default.
func terraformRootPath(configuredRoot string) string {
	if configuredRoot != "" {
		return configuredRoot
	}
	return defaultTerraformRoot
}

// getGlobalTerraformPaths returns the terraform paths, allowing override for testing.
func getGlobalTerraformPaths(configuredRoot string) (dir, binary, marker string) {
	if testDir := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR"); testDir != "" {
		return testDir, filepath.Join(testDir, globalTerraformBinaryName), filepath.Join(testDir, globalTerraformMarkerName)
	}
	root := terraformRootPath(configuredRoot)
	dir = filepath.Join(root, globalTerraformDirName)
	return dir, filepath.Join(dir, globalTerraformBinaryName), filepath.Join(dir, globalTerraformMarkerName)
}

// getInstallerCurrentPath returns the path to the "current" symlink created by the
// Terraform installer API (rad terraform install). Allows override for testing.
func getInstallerCurrentPath(configuredRoot string) string {
	if testDir := os.Getenv("TERRAFORM_TEST_INSTALLER_DIR"); testDir != "" {
		return filepath.Join(testDir, installerCurrentSymlinkName)
	}
	root := terraformRootPath(configuredRoot)
	return filepath.Join(root, installerCurrentSymlinkName)
}

var (
	// Global mutex to synchronize terraform binary installation and access
	globalTerraformMutex sync.Mutex
	// Track if global terraform binary is initialized
	globalTerraformReady bool
	// Track the path of the verified terraform binary (installer or global)
	verifiedTerraformPath string
	// Track which terraform root the cache is valid for (to invalidate when root changes)
	verifiedTerraformRoot string
)

// Install installs Terraform using a global shared binary approach.
// It uses a global mutex to ensure thread-safe access to the shared Terraform binary.
// This approach prevents concurrent file system operations that were causing state lock errors.
func Install(ctx context.Context, installer *install.Installer, opts InstallOptions) (*tfexec.Terraform, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Use global shared binary approach with proper locking
	execPath, err := ensureGlobalTerraformBinary(ctx, installer, logger, opts.TerraformPath)
	if err != nil {
		return nil, err
	}

	// Create a new instance of tfexec.Terraform with the global shared binary
	tf, err := NewTerraform(ctx, opts.RootDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Terraform instance with global shared binary: %w", err)
	}

	// Configure Terraform logs
	configureTerraformLogs(ctx, tf, opts.LogLevel)

	return tf, nil
}

// ensureGlobalTerraformBinary ensures a global shared Terraform binary is available.
// Uses mutex-based locking to prevent race conditions during concurrent access.
//
// Binary lookup order:
// 1. Previously verified binary path (cached in memory, scoped to terraform root)
// 2. Installer API binary at <terraform root>/current (from `rad terraform install`)
// 3. Global shared binary at <terraform root>/.terraform-global/terraform
// 4. Download via hc-install as last resort
func ensureGlobalTerraformBinary(ctx context.Context, installer *install.Installer, logger logr.Logger, terraformRoot string) (string, error) {
	// Normalize the terraform root for consistent comparison
	effectiveRoot := terraformRootPath(terraformRoot)

	// Get dynamic paths (allows testing override)
	globalDir, globalBinary, globalMarker := getGlobalTerraformPaths(terraformRoot)
	installerCurrentPath := getInstallerCurrentPath(terraformRoot)

	// Lock global mutex to prevent concurrent access
	globalTerraformMutex.Lock()
	defer globalTerraformMutex.Unlock()

	// Invalidate cache if terraform root changed (supports multi-tenant or config changes)
	if verifiedTerraformRoot != effectiveRoot {
		if verifiedTerraformRoot != "" {
			logger.Info("Terraform root changed, invalidating cache",
				"previousRoot", verifiedTerraformRoot, "newRoot", effectiveRoot)
		}
		globalTerraformReady = false
		verifiedTerraformPath = ""
		verifiedTerraformRoot = ""
	}

	// If we already have a verified path, check if it's still valid
	if globalTerraformReady && verifiedTerraformPath != "" {
		// Check if installer symlink exists and what it points to
		installerTarget, installerErr := filepath.EvalSymlinks(installerCurrentPath)

		if installerErr == nil {
			// Installer symlink exists - verify cache matches current target
			if verifiedTerraformPath == installerTarget {
				logger.Info("Using previously verified Terraform binary", "path", verifiedTerraformPath)
				return verifiedTerraformPath, nil
			}
			// Symlink changed (user ran `rad terraform install` with different version)
			// Invalidate cache so we pick up the new version
			logger.Info("Installer symlink target changed, invalidating cache",
				"cached", verifiedTerraformPath, "current", installerTarget)
			globalTerraformReady = false
			verifiedTerraformPath = ""
		} else {
			// No installer symlink - use cached path if binary still exists
			if _, err := os.Stat(verifiedTerraformPath); err == nil {
				logger.Info("Using previously verified Terraform binary", "path", verifiedTerraformPath)
				return verifiedTerraformPath, nil
			}
			// Binary no longer exists, reset state
			logger.Info("Previously verified Terraform binary no longer exists, searching for new binary", "path", verifiedTerraformPath)
			globalTerraformReady = false
			verifiedTerraformPath = ""
		}
	}

	// Priority 1: Check for installer API binary at /terraform/current
	// This is a symlink created by `rad terraform install` pointing to the active version
	if installerBinary, err := checkInstallerBinary(ctx, installerCurrentPath, logger); err == nil {
		logger.Info("Using Terraform binary from installer API", "path", installerBinary)
		globalTerraformReady = true
		verifiedTerraformPath = installerBinary
		verifiedTerraformRoot = effectiveRoot
		return installerBinary, nil
	}

	// Priority 2: Check for pre-mounted global binary
	_, binaryExists := os.Stat(globalBinary)
	_, markerExists := os.Stat(globalMarker)

	if binaryExists == nil && markerExists == nil {
		logger.Info("Found pre-mounted global Terraform binary")

		if err := verifyBinaryWorks(ctx, globalDir, globalBinary); err == nil {
			logger.Info("Successfully verified pre-mounted global Terraform binary")
			globalTerraformReady = true
			verifiedTerraformPath = globalBinary
			verifiedTerraformRoot = effectiveRoot
			return globalBinary, nil
		} else {
			logger.Error(err, "Pre-mounted global Terraform binary verification failed")
		}
	}

	// Priority 3: Download and install Terraform via hc-install
	if err := downloadAndInstallTerraform(ctx, installer, globalDir, globalBinary, globalMarker, logger); err != nil {
		return "", err
	}

	globalTerraformReady = true
	verifiedTerraformPath = globalBinary
	verifiedTerraformRoot = effectiveRoot
	logger.Info("Global shared Terraform binary is ready")

	return globalBinary, nil
}

// checkInstallerBinary checks if a Terraform binary installed by the installer API exists
// and is functional. The installerCurrentPath is typically a symlink to the active version.
// Returns the resolved binary path if successful, or an error if not available.
func checkInstallerBinary(ctx context.Context, installerCurrentPath string, logger logr.Logger) (string, error) {
	// Resolve the symlink (if any) to get the actual binary path.
	binaryPath, err := filepath.EvalSymlinks(installerCurrentPath)
	if err != nil {
		return "", fmt.Errorf("installer current path not found or invalid: %w", err)
	}

	// Verify the binary exists
	if _, err := os.Stat(binaryPath); err != nil {
		return "", fmt.Errorf("installer binary not found at %s: %w", binaryPath, err)
	}

	// Get the directory containing the binary for tfexec working directory
	binaryDir := filepath.Dir(binaryPath)

	// Verify the binary works
	if err := verifyBinaryWorks(ctx, binaryDir, binaryPath); err != nil {
		logger.Error(err, "Installer API Terraform binary verification failed", "path", binaryPath)
		return "", fmt.Errorf("installer binary verification failed: %w", err)
	}

	logger.Info("Successfully verified Terraform binary from installer API", "path", binaryPath)
	return binaryPath, nil
}

// verifyBinaryWorks creates a Terraform instance and verifies it works by calling Version.
func verifyBinaryWorks(ctx context.Context, workingDir, binaryPath string) error {
	tf, err := tfexec.NewTerraform(workingDir, binaryPath)
	if err != nil {
		return fmt.Errorf("failed to create Terraform instance: %w", err)
	}

	_, _, err = tf.Version(ctx, false)
	if err != nil {
		return fmt.Errorf("terraform version check failed: %w", err)
	}

	return nil
}

// downloadAndInstallTerraform downloads and installs Terraform to the global location.
func downloadAndInstallTerraform(ctx context.Context, installer *install.Installer, globalDir, globalBinary, globalMarker string, logger logr.Logger) error {
	logger.Info("Downloading Terraform to global shared location")

	// Create global terraform directory
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return fmt.Errorf("failed to create global terraform directory: %w", err)
	}

	installStartTime := time.Now()
	execPath, err := installer.Ensure(ctx, []src.Source{
		&releases.LatestVersion{
			Product:    product.Terraform,
			InstallDir: globalDir,
		},
	})
	if err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
			[]attribute.KeyValue{
				metrics.TerraformVersionAttrKey.String("latest"),
				metrics.OperationStateAttrKey.String(metrics.FailedOperationState),
			},
		)
		return fmt.Errorf("failed to install Terraform to global location: %w", err)
	}

	metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
		[]attribute.KeyValue{
			metrics.TerraformVersionAttrKey.String("latest"),
			metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState),
		},
	)

	logger.Info(fmt.Sprintf("Terraform installed to global location: %q", execPath))

	// Copy to our standardized global path if different
	if execPath != globalBinary {
		if data, err := os.ReadFile(execPath); err == nil {
			if err := os.WriteFile(globalBinary, data, 0755); err != nil {
				return fmt.Errorf("failed to copy terraform binary to global path: %w", err)
			}
			logger.Info("Copied Terraform binary to standardized global path")
		} else {
			return fmt.Errorf("failed to read installed terraform binary: %w", err)
		}
	}

	// Verify installation with retries
	for attempt := 0; attempt <= installVerificationRetryCount; attempt++ {
		err = verifyBinaryWorks(ctx, globalDir, globalBinary)
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
			logger.Error(err, fmt.Sprintf("Failed to verify global Terraform installation. Retrying after %d seconds", installVerificationRetryDelaySecs))
			metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallVerificationDuration(ctx, installStartTime,
				[]attribute.KeyValue{
					metrics.TerraformVersionAttrKey.String("latest"),
					metrics.OperationStateAttrKey.String(metrics.FailedOperationState),
				},
			)
			time.Sleep(time.Duration(installVerificationRetryDelaySecs) * time.Second)
			continue
		}
		return fmt.Errorf("failed to verify global Terraform installation after %d attempts. Error: %s", installVerificationRetryCount, err.Error())
	}

	// Create marker file to indicate successful installation
	if err := os.WriteFile(globalMarker, []byte("ready"), 0644); err != nil {
		return fmt.Errorf("failed to create global terraform marker file: %w", err)
	}

	return nil
}

// resetGlobalStateForTesting resets the global terraform state for testing purposes
// This should only be used in tests
func resetGlobalStateForTesting() {
	globalTerraformMutex.Lock()
	defer globalTerraformMutex.Unlock()
	globalTerraformReady = false
	verifiedTerraformPath = ""
	verifiedTerraformRoot = ""
}
