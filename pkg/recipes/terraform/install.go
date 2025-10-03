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

	// Global shared terraform binary paths (persistent hidden directory under terraform root)
	// Using .terraform-global as a more recognizable and persistent directory name
	defaultGlobalTerraformDir    = "/terraform/.terraform-global"
	defaultGlobalTerraformBinary = "/terraform/.terraform-global/terraform"
	defaultGlobalMarkerFile      = "/terraform/.terraform-global/.terraform-ready"
)

// getGlobalTerraformPaths returns the terraform paths, allowing override for testing
func getGlobalTerraformPaths() (dir, binary, marker string) {
	if testDir := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR"); testDir != "" {
		return testDir, testDir + "/terraform", testDir + "/.terraform-ready"
	}
	return defaultGlobalTerraformDir, defaultGlobalTerraformBinary, defaultGlobalMarkerFile
}

var (
	// Global mutex to synchronize terraform binary installation and access
	globalTerraformMutex sync.Mutex
	// Track if global terraform binary is initialized
	globalTerraformReady bool
)

// Install installs Terraform using a global shared binary approach.
// It uses a global mutex to ensure thread-safe access to the shared Terraform binary.
// This approach prevents concurrent file system operations that were causing state lock errors.
func Install(ctx context.Context, installer *install.Installer, tfDir string, logLevel string) (*tfexec.Terraform, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Use global shared binary approach with proper locking
	execPath, err := ensureGlobalTerraformBinary(ctx, installer, logger)
	if err != nil {
		return nil, err
	}

	// Create a new instance of tfexec.Terraform with the global shared binary
	tf, err := NewTerraform(ctx, tfDir, execPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create Terraform instance with global shared binary: %w", err)
	}

	// Configure Terraform logs
	configureTerraformLogs(ctx, tf, logLevel)

	return tf, nil
}

// ensureGlobalTerraformBinary ensures a global shared Terraform binary is available.
// Uses mutex-based locking to prevent race conditions during concurrent access.
func ensureGlobalTerraformBinary(ctx context.Context, installer *install.Installer, logger logr.Logger) (string, error) {
	// Get dynamic paths (allows testing override)
	globalDir, globalBinary, globalMarker := getGlobalTerraformPaths()

	// Lock global mutex to prevent concurrent access
	globalTerraformMutex.Lock()
	defer globalTerraformMutex.Unlock()

	_, binaryExists := os.Stat(globalBinary)
	_, markerExists := os.Stat(globalMarker)

	// If globalTerraformReady is true and both files exist, use existing binary
	if globalTerraformReady && binaryExists == nil && markerExists == nil {
		logger.Info("Using existing global shared Terraform binary")
		return globalBinary, nil
	}

	// If files are missing but globalTerraformReady was true, log and reset
	if globalTerraformReady {
		if binaryExists != nil {
			logger.Info(fmt.Sprintf("Global binary missing at %s, will reinstall", globalBinary))
		}
		if markerExists != nil {
			logger.Info(fmt.Sprintf("Global marker file missing at %s, will reinstall", globalMarker))
		}
		globalTerraformReady = false
	}

	// Check if pre-mounted binary exists and works
	if binaryExists == nil && markerExists == nil {
		logger.Info("Found pre-mounted global Terraform binary")

		if err := verifyBinaryWorks(ctx, globalDir, globalBinary); err == nil {
			logger.Info("Successfully verified pre-mounted global Terraform binary")
			globalTerraformReady = true
			return globalBinary, nil
		} else {
			logger.Error(err, "Pre-mounted global Terraform binary verification failed")
		}
	}

	// Download and install Terraform
	if err := downloadAndInstallTerraform(ctx, installer, globalDir, globalBinary, globalMarker, logger); err != nil {
		return "", err
	}

	globalTerraformReady = true
	logger.Info("Global shared Terraform binary is ready")

	return globalBinary, nil
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
}
