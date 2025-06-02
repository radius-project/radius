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
)

// CustomConfigValidationCheck validates that any custom configuration parameters
// (--set, --set-file) are well-formed and safe for the upgrade process.
type CustomConfigValidationCheck struct {
	setParams     []string
	setFileParams []string
}

// NewCustomConfigValidationCheck creates a new custom configuration validation check.
func NewCustomConfigValidationCheck(setParams, setFileParams []string) *CustomConfigValidationCheck {
	return &CustomConfigValidationCheck{
		setParams:     setParams,
		setFileParams: setFileParams,
	}
}

// Name returns the name of this check.
func (c *CustomConfigValidationCheck) Name() string {
	return "Custom Configuration Validation"
}

// Severity returns the severity level of this check.
func (c *CustomConfigValidationCheck) Severity() CheckSeverity {
	return SeverityWarning // Warnings don't block upgrades, just inform the user
}

// Run executes the custom configuration validation check.
func (c *CustomConfigValidationCheck) Run(ctx context.Context) (bool, string, error) {
	var issues []string

	// Validate --set parameters
	for _, param := range c.setParams {
		if issue := c.validateSetParameter(param); issue != "" {
			issues = append(issues, fmt.Sprintf("--set parameter '%s': %s", param, issue))
		}
	}

	// Validate --set-file parameters
	for _, param := range c.setFileParams {
		if issue := c.validateSetFileParameter(param); issue != "" {
			issues = append(issues, fmt.Sprintf("--set-file parameter '%s': %s", param, issue))
		}
	}

	// Check for potentially dangerous configuration overrides (warnings only)
	dangerousConfigs := c.findDangerousConfigurations()

	if len(issues) > 0 {
		return false, fmt.Sprintf("Configuration validation failed: %s", strings.Join(issues, "; ")), nil
	}

	configCount := len(c.setParams) + len(c.setFileParams)
	if configCount == 0 {
		return true, "No custom configuration parameters provided", nil
	}

	// Build success message
	message := fmt.Sprintf("All %d custom configuration parameters are valid", configCount)
	if len(dangerousConfigs) > 0 {
		message += fmt.Sprintf(". Warnings: %s", strings.Join(dangerousConfigs, "; "))
	}

	return true, message, nil
}

// validateSetParameter validates a single --set parameter.
func (c *CustomConfigValidationCheck) validateSetParameter(param string) string {
	// Check basic format (key=value)
	parts := strings.SplitN(param, "=", 2)
	if len(parts) != 2 {
		return "must be in format 'key=value'"
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	if key == "" {
		return "key cannot be empty"
	}

	if value == "" {
		return "value cannot be empty"
	}

	// Validate key format (should be valid Helm path)
	if !c.isValidHelmPath(key) {
		return "key contains invalid characters for Helm configuration path"
	}

	// Check for array/map syntax and validate if present
	if strings.Contains(key, "[") || strings.Contains(key, "]") {
		if !c.isValidArrayOrMapSyntax(key) {
			return "invalid array or map syntax in key"
		}
	}

	return ""
}

// validateSetFileParameter validates a single --set-file parameter.
func (c *CustomConfigValidationCheck) validateSetFileParameter(param string) string {
	// Check basic format (key=filepath)
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

	// Validate key format (should be valid Helm path)
	if !c.isValidHelmPath(key) {
		return "key contains invalid characters for Helm configuration path"
	}

	// Check for potentially dangerous file paths
	if c.isDangerousFilePath(filepath) {
		return "filepath appears to reference system files or use dangerous patterns"
	}

	return ""
}

// isValidHelmPath checks if a string is a valid Helm configuration path.
func (c *CustomConfigValidationCheck) isValidHelmPath(path string) bool {
	// Helm paths should contain only alphanumeric characters, dots, dashes, underscores, and brackets
	for _, char := range path {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '.' && char != '-' && char != '_' &&
			char != '[' && char != ']' {
			return false
		}
	}
	return true
}

// isValidArrayOrMapSyntax validates array/map bracket syntax in Helm paths.
func (c *CustomConfigValidationCheck) isValidArrayOrMapSyntax(path string) bool {
	bracketCount := 0
	inBrackets := false

	for i, char := range path {
		switch char {
		case '[':
			if inBrackets {
				return false // Nested brackets not allowed
			}
			inBrackets = true
			bracketCount++
		case ']':
			if !inBrackets {
				return false // Closing bracket without opening
			}
			inBrackets = false

			// Check if there's content between brackets
			if i > 0 && path[i-1] == '[' {
				return false // Empty brackets
			}
		}
	}

	// All brackets must be closed
	return !inBrackets && bracketCount > 0
}

// isDangerousFilePath checks if a file path might be dangerous.
func (c *CustomConfigValidationCheck) isDangerousFilePath(filepath string) bool {
	dangerous := []string{
		"/etc/", "/usr/", "/bin/", "/sbin/", "/var/",
		"../", "./", "~", "$",
	}

	lowerPath := strings.ToLower(filepath)
	for _, pattern := range dangerous {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}

	return false
}

// findDangerousConfigurations identifies potentially dangerous configuration overrides.
func (c *CustomConfigValidationCheck) findDangerousConfigurations() []string {
	var warnings []string

	dangerousKeys := map[string]string{
		"image":                   "overriding container images can introduce security vulnerabilities",
		"securityContext":         "security context changes can affect cluster security",
		"serviceAccount":          "service account changes can affect permissions",
		"rbac":                    "RBAC changes can affect cluster permissions",
		"nodeSelector":            "node selector changes can affect scheduling",
		"tolerations":             "toleration changes can affect scheduling",
		"affinity":                "affinity changes can affect scheduling",
		"resources.limits.cpu":    "CPU limit changes can affect cluster stability",
		"resources.limits.memory": "memory limit changes can affect cluster stability",
	}

	// Check all --set parameters for dangerous keys
	for _, param := range c.setParams {
		parts := strings.SplitN(param, "=", 2)
		if len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			for dangerousKey, warning := range dangerousKeys {
				if strings.Contains(key, dangerousKey) {
					warnings = append(warnings, fmt.Sprintf("potentially dangerous configuration '%s': %s", key, warning))
				}
			}
		}
	}

	return warnings
}
