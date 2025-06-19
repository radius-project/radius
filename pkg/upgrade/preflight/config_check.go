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

package preflight

import (
	"context"
	"fmt"
	"strings"

	"maps"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/helm"
	"helm.sh/helm/v3/pkg/strvals"
)

// CustomConfigValidationCheck validates that custom configuration parameters
// are accessible and properly formatted against the actual Helm chart.
//
// This check loads the Radius chart and validates --set parameters using Helm's
// own validation logic to ensure they match the chart's expected structure.
type CustomConfigValidationCheck struct {
	setParams     []string
	setFileParams []string
	chartPath     string
	helmClient    helm.HelmClient
	fs            filesystem.FileSystem
}

// NewCustomConfigValidationCheck creates a new custom configuration validation check.
// If chartPath is empty, it defaults to the standard Radius chart location.
// If helmClient is nil, a new client will be created.
// The check will fall back to basic syntax validation if the chart is not found.
func NewCustomConfigValidationCheck(setParams, setFileParams []string, chartPath string, helmClient helm.HelmClient) *CustomConfigValidationCheck {
	// Use default chart path if not specified
	if chartPath == "" {
		// Default path from pkg/upgrade/preflight to deploy/Chart
		chartPath = "../../../deploy/Chart"
	}

	// Create helm client if not provided
	if helmClient == nil {
		helmClient = helm.NewHelmClient()
	}

	return &CustomConfigValidationCheck{
		setParams:     setParams,
		setFileParams: setFileParams,
		chartPath:     chartPath,
		helmClient:    helmClient,
		fs:            filesystem.NewOSFS(),
	}
}

// Name returns the name of this check.
func (c *CustomConfigValidationCheck) Name() string {
	return "Custom Configuration Validation"
}

// Severity returns the severity level of this check.
func (c *CustomConfigValidationCheck) Severity() CheckSeverity {
	return SeverityWarning
}

// Run executes the custom configuration validation check.
func (c *CustomConfigValidationCheck) Run(ctx context.Context) (bool, string, error) {
	configCount := len(c.setParams) + len(c.setFileParams)
	if configCount == 0 {
		return true, "No custom configuration parameters provided", nil
	}

	// Validate basic format first
	if issues := c.validateAllParams(); len(issues) > 0 {
		return false, fmt.Sprintf("Configuration validation failed: %s", strings.Join(issues, "; ")), nil
	}

	// Try chart validation if available
	if issues := c.validateAgainstChart(); len(issues) > 0 {
		return false, fmt.Sprintf("Chart validation failed: %s", strings.Join(issues, "; ")), nil
	}

	// Determine validation type for message
	validationType := "basic validation"
	if c.chartPath != "" {
		validationType = "validation against Helm chart"
	}

	return true, fmt.Sprintf("All %d custom configuration parameters passed %s", configCount, validationType), nil
}

// validateParam validates basic format for parameters.
func (c *CustomConfigValidationCheck) validateParam(param, format string) string {
	parts := strings.SplitN(param, "=", 2)
	if len(parts) != 2 {
		return fmt.Sprintf("must be in format '%s'", format)
	}

	if strings.TrimSpace(parts[0]) == "" {
		return "key cannot be empty"
	}

	return ""
}

// validateFileParam validates --set-file parameters including file accessibility.
func (c *CustomConfigValidationCheck) validateFileParam(param string) string {
	if issue := c.validateParam(param, "key=filepath"); issue != "" {
		return issue
	}

	parts := strings.SplitN(param, "=", 2)
	filepath := strings.TrimSpace(parts[1])

	if filepath == "" {
		return "filepath cannot be empty"
	}

	if _, err := c.fs.ReadFile(filepath); err != nil {
		return fmt.Sprintf("cannot read file: %v", err)
	}

	return ""
}

// validateAgainstChart validates --set parameters against the actual Helm chart.
func (c *CustomConfigValidationCheck) validateAgainstChart() []string {
	if c.chartPath == "" {
		return nil // No chart path, skip validation
	}

	// Check chart accessibility and load
	if _, err := c.fs.Stat(c.chartPath); err != nil {
		return []string{fmt.Sprintf("failed to access chart path '%s': %v", c.chartPath, err)}
	}

	helmChart, err := c.helmClient.LoadChart(c.chartPath)
	if err != nil {
		return []string{fmt.Sprintf("failed to load chart from '%s': %v", c.chartPath, err)}
	}

	// Prepare test values
	testValues := make(map[string]any)
	if helmChart.Values != nil {
		maps.Copy(testValues, helmChart.Values)
	}

	var issues []string

	// Validate --set parameters
	for _, param := range c.setParams {
		if err := strvals.ParseInto(param, testValues); err != nil {
			issues = append(issues, fmt.Sprintf("--set parameter '%s' failed chart validation: %v", param, err))
		}
	}

	// Validate --set-file parameters
	reader := func(rs []rune) (any, error) {
		data, err := c.fs.ReadFile(string(rs))
		return string(data), err
	}

	for _, param := range c.setFileParams {
		if err := strvals.ParseIntoFile(param, testValues, reader); err != nil {
			issues = append(issues, fmt.Sprintf("--set-file parameter '%s' failed chart validation: %v", param, err))
		}
	}

	return issues
}

// validateAllParams validates both --set and --set-file parameters.
func (c *CustomConfigValidationCheck) validateAllParams() []string {
	var issues []string

	// Validate --set parameters
	for _, param := range c.setParams {
		if issue := c.validateParam(param, "key=value"); issue != "" {
			issues = append(issues, fmt.Sprintf("--set parameter '%s': %s", param, issue))
		}
	}

	// Validate --set-file parameters
	for _, param := range c.setFileParams {
		if issue := c.validateFileParam(param); issue != "" {
			issues = append(issues, fmt.Sprintf("--set-file parameter '%s': %s", param, issue))
		}
	}

	return issues
}
