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
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/otel/attribute"
)

const (
	installSubDir     = "install"
	installRetryCount = 5
	retryDelaySecs    = 3
)

// Install installs Terraform under /install in the provided Terraform root directory for the resource. It installs
// the latest version of Terraform and returns the path to the installed Terraform executable. It returns an error
// if the directory creation or Terraform installation fails.
func Install(ctx context.Context, installer *install.Installer, tfDir string) (*tfexec.Terraform, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	// Create Terraform installation directory
	installDir := filepath.Join(tfDir, installSubDir)
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory for terraform installation for resource: %w", err)
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
	for attempt := 0; attempt <= installRetryCount; attempt++ {
		errMsg := "Failed to verify Terraform installation: %s. Retrying after %d seconds"
		_, _, err = tf.Version(ctx, false)
		if err != nil {
			if attempt < installRetryCount {
				logger.Info(fmt.Sprintf(errMsg, err.Error(), retryDelaySecs))
				time.Sleep(time.Duration(retryDelaySecs) * time.Second)
				continue
			}
			return nil, fmt.Errorf(errMsg, err.Error(), retryDelaySecs)
		}
	}

	configureTerraformLogs(ctx, tf)

	return tf, nil
}
