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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCustomConfigValidationCheck_Run(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name              string
		setParams         []string
		setFileParams     []string
		setupFiles        func(t *testing.T) []string // returns files to cleanup
		chartPath         string                      // defaults to "../../../deploy/Chart" if empty
		shouldFail        bool                        // true if test should fail
		expectMsgContains []string                    // message fragments to check
	}{
		{
			name:              "no parameters",
			expectMsgContains: []string{"No custom configuration parameters provided"},
		},
		{
			name:              "valid set parameter",
			setParams:         []string{"app.config=value"},
			expectMsgContains: []string{"All 1 custom configuration parameters passed"},
		},
		{
			name:              "valid set parameter with complex key",
			setParams:         []string{"image.tag=v1.0.0"},
			expectMsgContains: []string{"All 1 custom configuration parameters passed"},
		},
		{
			name: "valid set-file parameter",
			setupFiles: func(t *testing.T) []string {
				tmpFile := createTempFile(t, "test content")
				return []string{fmt.Sprintf("values.yaml=%s", tmpFile)}
			},
			expectMsgContains: []string{"All 1 custom configuration parameters passed"},
		},
		{
			name:              "invalid set parameter format",
			setParams:         []string{"invalid-format"},
			shouldFail:        true,
			expectMsgContains: []string{"Configuration validation failed", "must be in format 'key=value'"},
		},
		{
			name:              "empty key in set parameter",
			setParams:         []string{"=value"},
			shouldFail:        true,
			expectMsgContains: []string{"key cannot be empty"},
		},
		{
			name:              "nonexistent file",
			setFileParams:     []string{"config=/nonexistent/file.yaml"},
			shouldFail:        true,
			expectMsgContains: []string{"file does not exist"},
		},
		{
			name:              "multiple parameters with mixed results",
			setParams:         []string{"valid.key=value", "invalid-format"},
			setFileParams:     []string{"config=/nonexistent/file.yaml"},
			shouldFail:        true,
			expectMsgContains: []string{"Configuration validation failed", "must be in format 'key=value'", "file does not exist"},
		},
		// Chart validation tests (uses default chart path)
		{
			name: "chart validation - valid parameters",
			setParams: []string{
				"de.image=ghcr.io/radius-project/deployment-engine:v1.0.0",
				"controller.resources.limits.memory=400Mi",
				"global.prometheus.enabled=false",
			},
			expectMsgContains: []string{"validation against Helm chart"},
		},
		{
			name: "chart validation - invalid parameter syntax",
			setParams: []string{
				"de.image[invalid=syntax",
			},
			shouldFail:        true,
			expectMsgContains: []string{"Chart validation failed", "failed chart validation"},
		},
		{
			name: "chart validation - valid complex paths",
			setParams: []string{
				"database.enabled=true",
				"database.postgres_user=testuser",
				"rp.resources.requests.memory=200Mi",
				"global.azureWorkloadIdentity.enabled=true",
			},
			expectMsgContains: []string{"validation against Helm chart"},
		},
		{
			name: "chart validation - set-file validation",
			setupFiles: func(t *testing.T) []string {
				configFile := createTempFile(t, "custom-config-content")
				return []string{fmt.Sprintf("global.rootCA.cert=%s", configFile)}
			},
			expectMsgContains: []string{"validation against Helm chart"},
		},
		{
			name: "chart validation - set-file with invalid key syntax for helm",
			setupFiles: func(t *testing.T) []string {
				configFile := createTempFile(t, "test content")
				// This will pass basic validation but fail Helm's parser
				// due to invalid array index syntax
				return []string{fmt.Sprintf("values[invalid].key=%s", configFile)}
			},
			shouldFail:        true,
			expectMsgContains: []string{"Chart validation failed", "--set-file parameter 'values[invalid].key=", "failed chart validation"},
		},
		{
			name:              "chart validation - nonexistent chart path",
			setParams:         []string{"de.image=test"},
			chartPath:         "/nonexistent/chart/path",
			shouldFail:        true,
			expectMsgContains: []string{"Chart validation failed", "chart path '/nonexistent/chart/path' does not exist"},
		},
		{
			name: "chart validation - non-existent key",
			setParams: []string{
				"foo=bar",
				"nonexistent.key=value",
			},
			expectMsgContains: []string{"validation against Helm chart"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFileParams := tt.setFileParams

			// Handle file setup if needed
			if tt.setupFiles != nil {
				fileParams := tt.setupFiles(t)
				setFileParams = fileParams
			}

			// Skip test if using default chart and it doesn't exist
			if tt.chartPath == "" {
				if _, err := os.Stat("../../../deploy/Chart"); os.IsNotExist(err) {
					t.Skip("Radius chart not found, skipping chart validation test")
				}
			}

			check := NewCustomConfigValidationCheck(tt.setParams, setFileParams, tt.chartPath, nil)
			pass, msg, err := check.Run(ctx)

			require.NoError(t, err)
			assert.Equal(t, !tt.shouldFail, pass)

			for _, contains := range tt.expectMsgContains {
				assert.Contains(t, msg, contains)
			}
		})
	}
}

func TestCustomConfigValidationCheck_ValidateSetParam(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{}, "", nil)

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
			name:      "valid complex key",
			param:     "app.env[0].name=TEST",
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
			name:      "empty value is allowed",
			param:     "key=",
			expectErr: "", // Empty values are allowed in Helm
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := check.validateSetParam(tt.param)
			if tt.expectErr == "" {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, tt.expectErr)
			}
		})
	}
}

func TestCustomConfigValidationCheck_ValidateSetFileParam(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{}, "", nil)

	tests := []struct {
		name      string
		param     string
		setupFile func(t *testing.T) string // returns the param to use
		expectErr string
	}{
		{
			name: "valid file parameter",
			setupFile: func(t *testing.T) string {
				tmpFile := createTempFile(t, "test content")
				return fmt.Sprintf("config=%s", tmpFile)
			},
			expectErr: "",
		},
		{
			name:      "invalid format",
			param:     "invalid",
			expectErr: "must be in format 'key=filepath'",
		},
		{
			name:      "empty key",
			param:     "=/path/to/file",
			expectErr: "key cannot be empty",
		},
		{
			name:      "empty filepath",
			param:     "config=",
			expectErr: "filepath cannot be empty",
		},
		{
			name:      "nonexistent file",
			param:     "config=/nonexistent/file.yaml",
			expectErr: "file does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := tt.param
			if tt.setupFile != nil {
				param = tt.setupFile(t)
			}

			result := check.validateSetFileParam(param)
			if tt.expectErr == "" {
				assert.Empty(t, result)
			} else {
				assert.Contains(t, result, tt.expectErr)
			}
		})
	}
}

// createTempFile creates a temporary file with the given content for testing.
func createTempFile(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	return tmpFile
}

func TestCustomConfigValidationCheck_LoadChartError(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a valid chart directory
	chartPath := t.TempDir()
	// Create Chart.yaml to make it look like a valid chart directory
	chartYaml := filepath.Join(chartPath, "Chart.yaml")
	err := os.WriteFile(chartYaml, []byte("name: test\nversion: 1.0.0"), 0644)
	require.NoError(t, err)

	mockClient := helm.NewMockHelmClient(ctrl)
	// Mock LoadChart to return an error
	mockClient.EXPECT().LoadChart(chartPath).Return(nil, errors.New("chart is corrupted"))

	check := NewCustomConfigValidationCheck(
		[]string{"key=value"},
		nil,
		chartPath,
		mockClient,
	)

	pass, msg, err := check.Run(ctx)

	require.NoError(t, err) // Run should not return error, just validation failure
	assert.False(t, pass)
	assert.Contains(t, msg, "Chart validation failed")
	assert.Contains(t, msg, "failed to load chart from")
	assert.Contains(t, msg, "chart is corrupted")
}
