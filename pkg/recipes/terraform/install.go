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
	"time"

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
)

// Install installs Terraform under /install in the provided Terraform root directory for the resource.
// It first checks for a pre-mounted Terraform binary (typically copied by an init container from a
// container image) at the root of the tfDir. If found and working, it uses the pre-mounted binary
// for better performance and reduced download overhead. If not found or not working, it falls back
// to downloading the latest version of Terraform and returns the path to the installed Terraform
// executable. It returns an error if the directory creation or Terraform installation fails.
func Install(ctx context.Context, installer *install.Installer, tfDir string) (*tfexec.Terraform, error) {
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
			configureTerraformLogs(ctx, tf)
			return tf, nil
		}
	}

	// Fall back to downloading Terraform if no pre-mounted binary is found or if it's not working
	logger.Info("No pre-mounted Terraform binary found, downloading Terraform...")

	// Create Terraform installation directory
	installDir := filepath.Join(tfDir, installSubDir)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for terraform installation for resource: %w", err)
	}

	// Check if pre-downloaded Terraform binary exists and copy it to installDir
	preMountedBinaryPath := "/terraform/terraform"
	markerFile := "/terraform/.terraform-source"
	
	if _, err := os.Stat(preMountedBinaryPath); err == nil {
		if _, err := os.Stat(markerFile); err == nil {
			logger.Info("Copying pre-downloaded Terraform binary to install directory")
			if data, err := os.ReadFile(preMountedBinaryPath); err == nil {
				if err := os.WriteFile(filepath.Join(installDir, "terraform"), data, 0755); err == nil {
					logger.Info("Successfully copied pre-downloaded Terraform binary")
				}
			}
		}
	}

	logger.Info(fmt.Sprintf("Installing Terraform in the directory: %q", installDir))

	installStartTime := time.Now()
	// Re-visit this: consider checking if an existing installation of same version of Terraform is available.
	// For initial iteration we will always install Terraform for every execution of the recipe driver.
	execPath, err := installer.Ensure(ctx, []src.Source{
		&releases.LatestVersion{
			Product:    product.Terraform,
			InstallDir: installDir,
		},
	})
	if err != nil {
		metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
			[]attribute.KeyValue{
				metrics.TerraformVersionAttrKey.String("latest"),
				metrics.OperationStateAttrKey.String(metrics.FailedOperationState),
			},
		)
		return nil, err
	}

	metrics.DefaultRecipeEngineMetrics.RecordTerraformInstallationDuration(ctx, installStartTime,
		[]attribute.KeyValue{
			metrics.TerraformVersionAttrKey.String("latest"),
			metrics.OperationStateAttrKey.String(metrics.SuccessfulOperationState),
		},
	)

	logger.Info(fmt.Sprintf("Terraform latest version installed to: %q", execPath))

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
	configureTerraformLogs(ctx, tf)

	return tf, nil
}
