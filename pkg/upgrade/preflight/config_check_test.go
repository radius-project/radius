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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCustomConfigValidationCheck_Properties(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	assert.Equal(t, "Custom Configuration Validation", check.Name())
	assert.Equal(t, SeverityWarning, check.Severity())
}

func TestCustomConfigValidationCheck_Run(t *testing.T) {
	tests := []struct {
		name          string
		setParams     []string
		setFileParams []string
		expectPass    bool
		expectMessage string
	}{
		{
			name:          "no parameters",
			setParams:     []string{},
			setFileParams: []string{},
			expectPass:    true,
			expectMessage: "No custom configuration parameters provided",
		},
		{
			name:          "valid set parameter with warning",
			setParams:     []string{"image.tag=v1.0.0"},
			setFileParams: []string{},
			expectPass:    true,
			expectMessage: "All 1 custom configuration parameters are valid. Warnings:",
		},
		{
			name:          "valid set parameter without warning",
			setParams:     []string{"app.config=value"},
			setFileParams: []string{},
			expectPass:    true,
			expectMessage: "All 1 custom configuration parameters are valid",
		},
		{
			name:          "valid set-file parameter",
			setParams:     []string{},
			setFileParams: []string{"values.yaml=/path/to/values.yaml"},
			expectPass:    true,
			expectMessage: "All 1 custom configuration parameters are valid",
		},
		{
			name:          "invalid set parameter format",
			setParams:     []string{"invalid-format"},
			setFileParams: []string{},
			expectPass:    false,
			expectMessage: "Configuration validation failed: --set parameter 'invalid-format': must be in format 'key=value'",
		},
		{
			name:          "empty key in set parameter",
			setParams:     []string{"=value"},
			setFileParams: []string{},
			expectPass:    false,
			expectMessage: "Configuration validation failed: --set parameter '=value': key cannot be empty",
		},
		{
			name:          "empty value in set parameter",
			setParams:     []string{"key="},
			setFileParams: []string{},
			expectPass:    false,
			expectMessage: "Configuration validation failed: --set parameter 'key=': value cannot be empty",
		},
		{
			name:          "dangerous file path",
			setParams:     []string{},
			setFileParams: []string{"config=/etc/passwd"},
			expectPass:    false,
			expectMessage: "Configuration validation failed: --set-file parameter 'config=/etc/passwd': filepath appears to reference system files or use dangerous patterns",
		},
		{
			name:          "valid array syntax",
			setParams:     []string{"env[0].name=TEST"},
			setFileParams: []string{},
			expectPass:    true,
			expectMessage: "All 1 custom configuration parameters are valid",
		},
		{
			name:          "invalid array syntax",
			setParams:     []string{"env[].name=TEST"},
			setFileParams: []string{},
			expectPass:    false,
			expectMessage: "Configuration validation failed: --set parameter 'env[].name=TEST': invalid array or map syntax in key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := NewCustomConfigValidationCheck(tt.setParams, tt.setFileParams)
			pass, message, err := check.Run(context.Background())

			assert.NoError(t, err)
			assert.Equal(t, tt.expectPass, pass)
			if tt.name == "valid set parameter with warning" {
				// For the warning case, just check that the base message is there and warnings are mentioned
				assert.Contains(t, message, "All 1 custom configuration parameters are valid")
				assert.Contains(t, message, "Warnings:")
			} else {
				assert.Contains(t, message, tt.expectMessage)
			}
		})
	}
}

func TestCustomConfigValidationCheck_ValidateSetParameter(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	tests := []struct {
		name      string
		param     string
		expectErr string
	}{
		{
			name:      "valid parameter",
			param:     "image.tag=v1.0.0",
			expectErr: "",
		},
		{
			name:      "missing equals",
			param:     "invalid",
			expectErr: "must be in format 'key=value'",
		},
		{
			name:      "empty key",
			param:     "=value",
			expectErr: "key cannot be empty",
		},
		{
			name:      "empty value",
			param:     "key=",
			expectErr: "value cannot be empty",
		},
		{
			name:      "invalid characters in key",
			param:     "key@invalid=value",
			expectErr: "key contains invalid characters for Helm configuration path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.validateSetParameter(tt.param)
			if tt.expectErr == "" {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, tt.expectErr)
			}
		})
	}
}

func TestCustomConfigValidationCheck_IsValidHelmPath(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	tests := []struct {
		name   string
		path   string
		expect bool
	}{
		{"simple key", "image", true},
		{"dotted path", "image.tag", true},
		{"with dashes", "container-name", true},
		{"with underscores", "env_var", true},
		{"with numbers", "port8080", true},
		{"with brackets", "env[0]", true},
		{"invalid symbols", "key@invalid", false},
		{"spaces", "key with spaces", false},
		{"slashes", "path/to/key", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.isValidHelmPath(tt.path)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestCustomConfigValidationCheck_IsValidArrayOrMapSyntax(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	tests := []struct {
		name   string
		path   string
		expect bool
	}{
		{"simple array", "env[0]", true},
		{"map access", "config[key]", true},
		{"nested path", "app.env[0].name", true},
		{"empty brackets", "env[]", false},
		{"unclosed bracket", "env[0", false},
		{"unopened bracket", "env0]", false},
		{"nested brackets", "env[[0]]", false},
		{"no brackets", "simple.path", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.isValidArrayOrMapSyntax(tt.path)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestCustomConfigValidationCheck_IsDangerousFilePath(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	tests := []struct {
		name   string
		path   string
		expect bool
	}{
		{"safe file", "/home/user/config.yaml", false},
		{"relative safe", "config/values.yaml", false},
		{"etc directory", "/etc/passwd", true},
		{"usr directory", "/usr/bin/something", true},
		{"parent directory", "../config", true},
		{"current directory", "./config", true},
		{"home shortcut", "~/config", true},
		{"variable", "$HOME/config", true},
		{"case insensitive", "/ETC/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.isDangerousFilePath(tt.path)
			assert.Equal(t, tt.expect, result)
		})
	}
}
