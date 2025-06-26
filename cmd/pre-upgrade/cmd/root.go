package cmd

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

import (
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preupgrade"
)

// RootCmd is the root command of the rad CLI. This is exported so we can generate docs for it.
var rootCmd = &cobra.Command{
	Use:   "pre-upgrade",
	Short: "Pre-upgrade service",
	Long:  `Pre-upgrade service for Radius, which performs checks before an upgrade.`,
}

func Execute() error {
	ctx := rootCmd.Context()

	config := preupgrade.Config{
		KubeContext: "", // Empty string for in-cluster configuration
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

	options := preupgrade.Options{
		EnabledChecks: enabledChecks,
		TargetVersion: targetVersion,
	}

	return preupgrade.RunPreflightChecks(ctx, config, options)
}
