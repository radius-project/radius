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
	"os"
	"strings"

	"maps"

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
	chartVersion  string
	helmClient    helm.HelmClient
}

// NewCustomConfigValidationCheck creates a new custom configuration validation check.
func NewCustomConfigValidationCheck(setParams, setFileParams []string) *CustomConfigValidationCheck {
	return &CustomConfigValidationCheck{
		setParams:     setParams,
		setFileParams: setFileParams,
		helmClient:    helm.NewHelmClient(),
	}
}

// NewCustomConfigValidationCheckWithChart creates a new custom configuration validation check
// with specific chart configuration and optional helm client for testing.
func NewCustomConfigValidationCheckWithChart(setParams, setFileParams []string, chartPath, chartVersion string, helmClient helm.HelmClient) *CustomConfigValidationCheck {
	client := helmClient
	if client == nil {
		client = helm.NewHelmClient()
	}

	return &CustomConfigValidationCheck{
		setParams:     setParams,
		setFileParams: setFileParams,
		chartPath:     chartPath,
		chartVersion:  chartVersion,
		helmClient:    client,
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

	var issues []string

	// Basic format validation for --set parameters
	for _, param := range c.setParams {
		if issue := c.validateSetParam(param); issue != "" {
			issues = append(issues, fmt.Sprintf("--set parameter '%s': %s", param, issue))
		}
	}

	// File existence validation for --set-file parameters
	for _, param := range c.setFileParams {
		if issue := c.validateSetFileParam(param); issue != "" {
			issues = append(issues, fmt.Sprintf("--set-file parameter '%s': %s", param, issue))
		}
	}

	// If basic validation failed, return early
	if len(issues) > 0 {
		return false, fmt.Sprintf("Configuration validation failed: %s", strings.Join(issues, "; ")), nil
	}

	// Perform chart-based validation if chart information is available
	if c.chartPath != "" {
		if chartIssues := c.validateAgainstChart(); len(chartIssues) > 0 {
			return false, fmt.Sprintf("Chart validation failed: %s", strings.Join(chartIssues, "; ")), nil
		}
	}

	// Build success message
	validationType := "basic validation"
	if c.chartPath != "" {
		validationType = "validation against Helm chart"
	}
	message := fmt.Sprintf("All %d custom configuration parameters passed %s", configCount, validationType)

	return true, message, nil
}

// validateSetParam performs basic format validation for --set parameters.
func (c *CustomConfigValidationCheck) validateSetParam(param string) string {
	parts := strings.SplitN(param, "=", 2)
	if len(parts) != 2 {
		return "must be in format 'key=value'"
	}

	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "key cannot be empty"
	}

	// Note: We intentionally don't validate the key format here since Helm's
	// validation rules are complex and may change. Let Helm handle that validation.

	return ""
}

// validateSetFileParam validates --set-file parameters and checks file accessibility.
func (c *CustomConfigValidationCheck) validateSetFileParam(param string) string {
	parts := strings.SplitN(param, "=", 2)
	if len(parts) != 2 {
		return "must be in format 'key=filepath'"
	}

	key := strings.TrimSpace(parts[0])
	filepath := strings.TrimSpace(parts[1])

	if key == "" {
		return "key cannot be empty"
	}

	if filepath == "" {
		return "filepath cannot be empty"
	}

	// Check if file exists and is readable
	if _, err := os.Stat(filepath); err != nil {
		if os.IsNotExist(err) {
			return "file does not exist"
		}
		return fmt.Sprintf("cannot access file: %v", err)
	}

	// Try to read the file to ensure it's accessible
	if _, err := os.ReadFile(filepath); err != nil {
		return fmt.Sprintf("cannot read file: %v", err)
	}

	return ""
}

// validateAgainstChart validates --set parameters against the actual Helm chart.
func (c *CustomConfigValidationCheck) validateAgainstChart() []string {
	var issues []string

	helmChart, err := c.helmClient.LoadChart(c.chartPath)
	if err != nil {
		issues = append(issues, fmt.Sprintf("failed to load chart from '%s': %v", c.chartPath, err))
		return issues
	}

	// Create a copy of the chart values to test parameter application
	testValues := make(map[string]any)
	if helmChart.Values != nil {
		maps.Copy(testValues, helmChart.Values)
	}

	// Apply --set parameters and validate them
	for _, param := range c.setParams {
		if err := strvals.ParseInto(param, testValues); err != nil {
			issues = append(issues, fmt.Sprintf("--set parameter '%s' failed chart validation: %v", param, err))
		}
	}

	// Apply --set-file parameters and validate them
	for _, param := range c.setFileParams {
		reader := func(rs []rune) (any, error) {
			data, err := os.ReadFile(string(rs))
			return string(data), err
		}

		if err := strvals.ParseIntoFile(param, testValues, reader); err != nil {
			issues = append(issues, fmt.Sprintf("--set-file parameter '%s' failed chart validation: %v", param, err))
		}
	}

	return issues
}
