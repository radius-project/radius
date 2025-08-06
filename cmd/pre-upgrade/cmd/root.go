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

package cmd

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preupgrade"
)

var rootCmd = &cobra.Command{
	Use:   "pre-upgrade",
	Short: "Pre-upgrade service",
	Long:  `Pre-upgrade service for Radius, which performs checks before an upgrade.`,
}

func Execute() error {
	ctx := rootCmd.Context()

	config := preupgrade.Config{
		Helm: &helm.Impl{
			Helm: helm.NewHelmClient(),
		},
		Output: &output.OutputWriter{
			Writer: rootCmd.OutOrStdout(),
		},
	}

	enabledChecksEnv := os.Getenv("ENABLED_CHECKS")
	enabledChecks := strings.Split(enabledChecksEnv, ",")

	targetVersion := os.Getenv("TARGET_VERSION")

	// Parse timeout from environment variable
	var timeout time.Duration
	timeoutEnv := os.Getenv("PREFLIGHT_TIMEOUT_SECONDS")
	if timeoutEnv != "" {
		seconds, err := strconv.Atoi(timeoutEnv)
		if err != nil {
			config.Output.LogInfo("Warning: Invalid PREFLIGHT_TIMEOUT_SECONDS value '%s', using default", timeoutEnv)
		} else {
			timeout = time.Duration(seconds) * time.Second
		}
	}

	// Retrieve current version from cluster for accurate logging
	var currentVersion string
	state, err := config.Helm.CheckRadiusInstall(config.KubeContext)
	if err != nil {
		config.Output.LogInfo("Warning: Failed to detect current Radius version: %v", err)
		currentVersion = "unknown"
	} else if !state.RadiusInstalled {
		currentVersion = "not-installed"
	} else {
		currentVersion = state.RadiusVersion
	}

	options := preupgrade.Options{
		EnabledChecks:  enabledChecks,
		TargetVersion:  targetVersion,
		CurrentVersion: currentVersion,
		Timeout:        timeout,
	}

	// Run preflight checks and ensure proper exit code
	err = preupgrade.RunPreflightChecks(ctx, config, options)
	if err != nil {
		config.Output.LogInfo("ERROR: %v", err)
		// Return error to propagate failure
		return err
	}

	return nil
}
