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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"helm.sh/helm/v3/pkg/chart"

	"github.com/radius-project/radius/pkg/cli/helm"
)

func TestCustomConfigValidationCheck_Properties(t *testing.T) {
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	assert.Equal(t, "Custom Configuration Validation", check.Name())
	assert.Equal(t, SeverityWarning, check.Severity())
}

func TestCustomConfigValidationCheck_Run(t *testing.T) {
	ctx := context.Background()

	t.Run("no parameters", func(t *testing.T) {
		check := NewCustomConfigValidationCheck([]string{}, []string{})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Equal(t, "No custom configuration parameters provided", msg)
	})

	t.Run("valid set parameter", func(t *testing.T) {
		check := NewCustomConfigValidationCheck([]string{"app.config=value"}, []string{})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "All 1 custom configuration parameters passed basic validation")
	})

	t.Run("valid set parameter with complex key", func(t *testing.T) {
		check := NewCustomConfigValidationCheck([]string{"image.tag=v1.0.0"}, []string{})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "All 1 custom configuration parameters passed basic validation")
	})

	t.Run("valid set-file parameter", func(t *testing.T) {
		// Create a temporary file
		tmpFile := createTempFile(t, "test content")
		defer func() { _ = os.Remove(tmpFile) }()

		check := NewCustomConfigValidationCheck([]string{}, []string{fmt.Sprintf("values.yaml=%s", tmpFile)})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "All 1 custom configuration parameters passed basic validation")
	})

	t.Run("invalid set parameter format", func(t *testing.T) {
		check := NewCustomConfigValidationCheck([]string{"invalid-format"}, []string{})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "Configuration validation failed")
		assert.Contains(t, msg, "must be in format 'key=value'")
	})

	t.Run("empty key in set parameter", func(t *testing.T) {
		check := NewCustomConfigValidationCheck([]string{"=value"}, []string{})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "key cannot be empty")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		check := NewCustomConfigValidationCheck([]string{}, []string{"config=/nonexistent/file.yaml"})
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "file does not exist")
	})

	t.Run("multiple parameters with mixed results", func(t *testing.T) {
		check := NewCustomConfigValidationCheck(
			[]string{"valid.key=value", "invalid-format"},
			[]string{"config=/nonexistent/file.yaml"},
		)
		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "Configuration validation failed")
		// Should contain multiple error messages
		assert.Contains(t, msg, "must be in format 'key=value'")
		assert.Contains(t, msg, "file does not exist")
	})
}

func TestCustomConfigValidationCheck_ValidateSetParam(t *testing.T) {
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
	check := NewCustomConfigValidationCheck([]string{}, []string{})

	t.Run("valid file parameter", func(t *testing.T) {
		tmpFile := createTempFile(t, "test content")
		defer func() { _ = os.Remove(tmpFile) }()

		result := check.validateSetFileParam(fmt.Sprintf("config=%s", tmpFile))
		assert.Empty(t, result)
	})

	t.Run("invalid format", func(t *testing.T) {
		result := check.validateSetFileParam("invalid")
		assert.Contains(t, result, "must be in format 'key=filepath'")
	})

	t.Run("empty key", func(t *testing.T) {
		result := check.validateSetFileParam("=/path/to/file")
		assert.Contains(t, result, "key cannot be empty")
	})

	t.Run("empty filepath", func(t *testing.T) {
		result := check.validateSetFileParam("config=")
		assert.Contains(t, result, "filepath cannot be empty")
	})

	t.Run("nonexistent file", func(t *testing.T) {
		result := check.validateSetFileParam("config=/nonexistent/file.yaml")
		assert.Contains(t, result, "file does not exist")
	})
}

// createTempFile creates a temporary file with the given content for testing.
func createTempFile(t *testing.T, content string) string {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.yaml")

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	require.NoError(t, err)

	return tmpFile
}

func TestCustomConfigValidationCheck_ChartValidation(t *testing.T) {
	ctx := context.Background()

	t.Run("chart validation - valid parameters", func(t *testing.T) {
		// Create a mock controller and Helm client
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockHelmClient := helm.NewMockHelmClient(ctrl)

		// Create a mock chart with realistic Radius values
		mockChart := &chart.Chart{
			Values: map[string]any{
				"de": map[string]any{
					"image": "ghcr.io/radius-project/deployment-engine",
					"tag":   "latest",
				},
				"controller": map[string]any{
					"image": "ghcr.io/radius-project/controller",
				},
				"global": map[string]any{
					"prometheus": map[string]any{
						"enabled": true,
					},
				},
			},
		}

		// Create a temporary chart directory
		tmpDir := t.TempDir()
		chartPath := filepath.Join(tmpDir, "radius-chart")
		err := os.MkdirAll(chartPath, 0755)
		require.NoError(t, err)

		// Set up mock expectations
		mockHelmClient.EXPECT().LoadChart(chartPath).Return(mockChart, nil)

		// Test valid parameters that exist in the chart
		check := NewCustomConfigValidationCheckWithChart(
			[]string{"de.image=ghcr.io/radius-project/deployment-engine:v1.0.0"},
			[]string{},
			chartPath,
			"v1.0.0",
			mockHelmClient,
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "validation against Helm chart")
	})

	t.Run("chart validation - invalid parameter path", func(t *testing.T) {
		// Create a mock controller and Helm client
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockHelmClient := helm.NewMockHelmClient(ctrl)

		// Create a mock chart with realistic Radius values
		mockChart := &chart.Chart{
			Values: map[string]any{
				"de": map[string]any{
					"image": "ghcr.io/radius-project/deployment-engine",
				},
			},
		}

		// Create a temporary chart directory
		tmpDir := t.TempDir()
		chartPath := filepath.Join(tmpDir, "radius-chart")
		err := os.MkdirAll(chartPath, 0755)
		require.NoError(t, err)

		// Set up mock expectations
		mockHelmClient.EXPECT().LoadChart(chartPath).Return(mockChart, nil)

		check := NewCustomConfigValidationCheckWithChart(
			[]string{"de.image[invalid=syntax"},
			[]string{},
			chartPath,
			"v1.0.0",
			mockHelmClient,
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "Chart validation failed")
		assert.Contains(t, msg, "failed chart validation")
	})

	t.Run("chart validation - chart load failure", func(t *testing.T) {
		// Create a mock controller and Helm client
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockHelmClient := helm.NewMockHelmClient(ctrl)

		chartPath := "/nonexistent/chart/path"

		// Set up mock expectations for chart load failure
		mockHelmClient.EXPECT().LoadChart(chartPath).Return(nil, fmt.Errorf("chart not found"))

		check := NewCustomConfigValidationCheckWithChart(
			[]string{"de.image=test"},
			[]string{},
			chartPath,
			"v1.0.0",
			mockHelmClient,
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "Chart validation failed")
		assert.Contains(t, msg, "failed to load chart")
	})

	t.Run("chart validation - set-file validation", func(t *testing.T) {
		// Create a mock controller and Helm client
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockHelmClient := helm.NewMockHelmClient(ctrl)

		// Create a mock chart
		mockChart := &chart.Chart{
			Values: map[string]any{
				"de": map[string]any{
					"config": "default-config",
				},
			},
		}

		// Create a temporary chart directory and config file
		tmpDir := t.TempDir()
		chartPath := filepath.Join(tmpDir, "radius-chart")
		err := os.MkdirAll(chartPath, 0755)
		require.NoError(t, err)

		configFile := createTempFile(t, "custom-config-content")
		defer func() { _ = os.Remove(configFile) }()

		// Set up mock expectations
		mockHelmClient.EXPECT().LoadChart(chartPath).Return(mockChart, nil)

		check := NewCustomConfigValidationCheckWithChart(
			[]string{},
			[]string{fmt.Sprintf("de.config=%s", configFile)},
			chartPath,
			"v1.0.0",
			mockHelmClient,
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "validation against Helm chart")
	})

	t.Run("fallback to basic validation when no chart path", func(t *testing.T) {
		// This should behave like the original basic validation
		check := NewCustomConfigValidationCheck(
			[]string{"de.image=test"},
			[]string{},
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "basic validation")
		assert.NotContains(t, msg, "chart")
	})
}

func TestCustomConfigValidationCheck_WithRealChart(t *testing.T) {
	ctx := context.Background()

	t.Run("integration test with real chart structure", func(t *testing.T) {
		// Skip this test if we're not in the radius repo or chart doesn't exist
		chartPath := "../../../deploy/Chart"
		if _, err := os.Stat(chartPath); os.IsNotExist(err) {
			t.Skip("Radius chart not found, skipping integration test")
		}

		check := NewCustomConfigValidationCheckWithChart(
			[]string{
				"de.image=ghcr.io/radius-project/deployment-engine:v1.0.0",
				"controller.image=ghcr.io/radius-project/controller:v1.0.0",
				"global.prometheus.enabled=false",
			},
			[]string{},
			chartPath,
			"",
			nil,
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.True(t, pass)
		assert.Contains(t, msg, "validation against Helm chart")
	})

	t.Run("integration test with invalid parameter", func(t *testing.T) {
		// Skip this test if we're not in the radius repo or chart doesn't exist
		chartPath := "../../../deploy/Chart"
		if _, err := os.Stat(chartPath); os.IsNotExist(err) {
			t.Skip("Radius chart not found, skipping integration test")
		}

		check := NewCustomConfigValidationCheckWithChart(
			[]string{
				"invalid.parameter[syntax=broken",
			},
			[]string{},
			chartPath,
			"",
			nil,
		)

		pass, msg, err := check.Run(ctx)

		require.NoError(t, err)
		assert.False(t, pass)
		assert.Contains(t, msg, "Chart validation failed")
	})
}
