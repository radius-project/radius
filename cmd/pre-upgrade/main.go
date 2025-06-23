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

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preflight"
)

type simpleOutput struct{}

type simpleStep struct{}

func (s *simpleOutput) LogInfo(format string, args ...any) {
	fmt.Printf(format, args...)
	fmt.Println()
}

func (s *simpleOutput) WriteFormatted(format string, obj any, options output.FormatterOptions) error {
	return fmt.Errorf("WriteFormatted not supported in pre-upgrade container")
}

func (s *simpleOutput) BeginStep(format string, args ...any) output.Step {
	fmt.Printf("Starting: ")
	fmt.Printf(format, args...)
	fmt.Println()
	return output.Step{}
}

func (s *simpleOutput) CompleteStep(step output.Step) {
}

func main() {
	ctx := context.Background()

	currentVersion := os.Getenv("CURRENT_VERSION")
	targetVersion := os.Getenv("TARGET_VERSION")
	enabledChecks := os.Getenv("ENABLED_CHECKS")

	if currentVersion == "" {
		fmt.Fprintf(os.Stderr, "Error: CURRENT_VERSION environment variable is required\n")
		os.Exit(1)
	}

	if targetVersion == "" {
		fmt.Fprintf(os.Stderr, "Error: TARGET_VERSION environment variable is required\n")
		os.Exit(1)
	}

	if enabledChecks == "" {
		enabledChecks = "version"
	}

	output := &simpleOutput{}
	registry := preflight.NewRegistry(output)

	checks := strings.Split(enabledChecks, ",")
	fmt.Printf("Running preflight checks: %s\n", strings.Join(checks, ", "))
	fmt.Printf("Current version: %s\n", currentVersion)
	fmt.Printf("Target version: %s\n", targetVersion)

	for _, checkName := range checks {
		checkName = strings.TrimSpace(checkName)

		switch checkName {
		case "version":
			versionCheck := preflight.NewVersionCompatibilityCheck(currentVersion, targetVersion)
			registry.AddCheck(versionCheck)

		case "resources":
			resourceCheck := preflight.NewKubernetesResourceCheck("")
			registry.AddCheck(resourceCheck)

		case "installation":
			helmClient := helm.NewHelmClient()
			helmInterface := &helm.Impl{
				Helm: helmClient,
			}

			installationCheck := preflight.NewRadiusInstallationCheck(helmInterface, "")
			registry.AddCheck(installationCheck)

		default:
			fmt.Fprintf(os.Stderr, "Error: Unknown check '%s'\n", checkName)
			os.Exit(1)
		}
	}

	results, err := registry.RunChecks(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Preflight checks failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("All preflight checks completed successfully\n")

	for _, result := range results {
		if result.Success {
			fmt.Printf("✓ %s: %s\n", result.Check.Name(), result.Message)
		} else if result.Severity == preflight.SeverityWarning {
			fmt.Printf("⚠ %s: %s\n", result.Check.Name(), result.Message)
		}
	}

	os.Exit(0)
}
