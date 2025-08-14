/*
Copyright 2025 The Radius Authors.

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

package preupgrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preflight"
)

// Config holds the configuration for running pre-upgrade checks
type Config struct {
	KubeContext string
	Helm        helm.Interface
	Output      output.Interface
}

// Options holds the options for preflight checks
type Options struct {
	EnabledChecks  []string
	TargetVersion  string
	CurrentVersion string
	Timeout        time.Duration // Timeout for all preflight checks combined, defaults to 1 minute if not set
}

// RunPreflightChecks executes all configured preflight checks
func RunPreflightChecks(ctx context.Context, config Config, options Options) error {
	// Apply default timeout if not set
	// Most checks complete quickly (version check, helm check), but we allow 1 minute
	// to account for potential network delays or slow Kubernetes API responses
	timeout := options.Timeout
	if timeout == 0 {
		timeout = 1 * time.Minute
	}

	// Create context with timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	registry := preflight.NewRegistry(config.Output)

	config.Output.LogInfo("Running preflight checks: %s", strings.Join(options.EnabledChecks, ", "))
	config.Output.LogInfo("Target version: %s", options.TargetVersion)
	config.Output.LogInfo("Current version: %s", options.CurrentVersion)

	for _, checkName := range options.EnabledChecks {
		checkName = strings.TrimSpace(checkName)

		if checkName == "" {
			continue
		}

		switch checkName {
		case "version":
			state, err := config.Helm.CheckRadiusInstall(config.KubeContext)
			if err != nil {
				return fmt.Errorf("failed to check current Radius installation: %w", err)
			}

			// If Radius is not installed, this is a new installation, not an upgrade
			// In this case, we can skip the version compatibility check
			if !state.RadiusInstalled {
				config.Output.LogInfo("Radius is not currently installed. Proceeding with fresh installation.")
				continue
			}

			currentVersion := state.RadiusVersion
			versionCheck := preflight.NewVersionCompatibilityCheck(currentVersion, options.TargetVersion)
			registry.AddCheck(versionCheck)

		case "helm":
			helmCheck := preflight.NewHelmConnectivityCheck(config.Helm, config.KubeContext)
			registry.AddCheck(helmCheck)

		case "installation":
			installationCheck := preflight.NewRadiusInstallationCheck(config.Helm, config.KubeContext)
			registry.AddCheck(installationCheck)

		case "kubernetes":
			kubernetesCheck := preflight.NewKubernetesConnectivityCheck(config.KubeContext)
			registry.AddCheck(kubernetesCheck)

		case "resources":
			resourcesCheck := preflight.NewKubernetesResourceCheck(config.KubeContext)
			registry.AddCheck(resourcesCheck)

		default:
			// Log warning but continue with other checks
			config.Output.LogInfo("Warning: Unknown check '%s', skipping", checkName)
		}
	}

	results, err := registry.RunChecks(ctxWithTimeout)
	if err != nil {
		// Check if the error was due to timeout
		if ctxWithTimeout.Err() == context.DeadlineExceeded {
			return fmt.Errorf("preflight checks timed out after %v", timeout)
		}
		return fmt.Errorf("preflight checks failed: %w", err)
	}

	config.Output.LogInfo("All preflight checks completed successfully")

	for _, result := range results {
		if result.Success {
			config.Output.LogInfo("✓ Success: %s: %s", result.Check.Name(), result.Message)
		} else if result.Severity == preflight.SeverityWarning {
			config.Output.LogInfo("⚠ Warning: %s: %s", result.Check.Name(), result.Message)
		}
	}

	return nil
}
