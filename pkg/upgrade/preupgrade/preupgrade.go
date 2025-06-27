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
}

// RunPreflightChecks executes all configured preflight checks
func RunPreflightChecks(ctx context.Context, config Config, options Options) error {
	registry := preflight.NewRegistry(config.Output)

	config.Output.LogInfo("Running preflight checks: %s", strings.Join(options.EnabledChecks, ", "))

	for _, checkName := range options.EnabledChecks {
		checkName = strings.TrimSpace(checkName)

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

		default:
			return fmt.Errorf("unknown check '%s'", checkName)
		}
	}

	results, err := registry.RunChecks(ctx)
	if err != nil {
		return fmt.Errorf("preflight checks failed: %w", err)
	}

	config.Output.LogInfo("All preflight checks completed successfully")

	for _, result := range results {
		if result.Success {
			config.Output.LogInfo("✓ %s: %s", result.Check.Name(), result.Message)
		} else if result.Severity == preflight.SeverityWarning {
			config.Output.LogInfo("⚠ %s: %s", result.Check.Name(), result.Message)
		}
	}

	return nil
}
