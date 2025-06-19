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

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCustomConfigValidationCheck_BasicValidation(t *testing.T) {
	tests := []struct {
		name              string
		setParams         []string
		setFileParams     []string
		setupFiles        func(t *testing.T) []string
		shouldFail        bool
		expectMsgContains []string
	}{
		{
			name: "no parameters",
			expectMsgContains: []string{
				"No custom configuration parameters provided",
			},
		},
		{
			name: "valid parameters - mixed types",
			setParams: []string{
				"app.config=value",
				"image.tag=v1.0.0",
			},
			setupFiles: func(t *testing.T) []string {
				tmpFile := createTempFile(t, "test content")
				return []string{fmt.Sprintf("values.yaml=%s", tmpFile)}
			},
			expectMsgContains: []string{
				"All 3 custom configuration parameters passed",
				"basic validation",
			},
		},
		{
			name: "invalid parameters - mixed failures",
			setParams: []string{
				"valid.key=value",
				"invalid-format",
				"=empty-key",
			},
			setFileParams: []string{
				"config=/nonexistent/file.yaml",
				"=empty-key-file",
			},
			shouldFail: true,
			expectMsgContains: []string{
				"Configuration validation failed",
				"must be in format 'key=value'",
				"key cannot be empty",
				"cannot read file",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFileParams := tt.setFileParams
			if tt.setupFiles != nil {
				setFileParams = tt.setupFiles(t)
			}

			check := NewCustomConfigValidationCheck(tt.setParams, setFileParams, "", nil)
			// Override the default chart path to skip chart validation for basic tests
			check.chartPath = ""
			pass, msg, err := check.Run(context.Background())

			require.NoError(t, err)
			assert.Equal(t, !tt.shouldFail, pass)
			for _, contains := range tt.expectMsgContains {
				assert.Contains(t, msg, contains)
			}
		})
	}
}

func TestCustomConfigValidationCheck_ChartValidation(t *testing.T) {
	// Skip if chart doesn't exist
	if _, err := os.Stat("../../../deploy/Chart"); os.IsNotExist(err) {
		t.Skip("Radius chart not found, skipping chart validation tests")
	}

	tests := []struct {
		name              string
		setParams         []string
		setFileParams     []string
		setupFiles        func(t *testing.T) []string
		chartPath         string
		shouldFail        bool
		expectMsgContains []string
	}{
		{
			name: "valid chart parameters",
			setParams: []string{
				"de.image=ghcr.io/radius-project/deployment-engine:v1.0.0",
				"controller.resources.limits.memory=400Mi",
				"global.prometheus.enabled=false",
				"database.enabled=true",
			},
			expectMsgContains: []string{
				"validation against Helm chart",
			},
		},
		{
			name: "invalid parameter syntax",
			setParams: []string{
				"de.image[invalid=syntax",
				"values[invalid].key=test",
			},
			shouldFail: true,
			expectMsgContains: []string{
				"Chart validation failed",
				"failed chart validation",
			},
		},
		{
			name: "valid set-file with chart",
			setupFiles: func(t *testing.T) []string {
				configFile := createTempFile(t, "custom-config-content")
				return []string{fmt.Sprintf("global.rootCA.cert=%s", configFile)}
			},
			expectMsgContains: []string{
				"validation against Helm chart",
			},
		},
		{
			name: "nonexistent chart path",
			setParams: []string{
				"de.image=test",
			},
			chartPath:  "/nonexistent/chart/path",
			shouldFail: true,
			expectMsgContains: []string{
				"Chart validation failed",
				"failed to access chart path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setFileParams := tt.setFileParams
			if tt.setupFiles != nil {
				setFileParams = tt.setupFiles(t)
			}

			check := NewCustomConfigValidationCheck(tt.setParams, setFileParams, tt.chartPath, nil)
			pass, msg, err := check.Run(context.Background())

			require.NoError(t, err)
			assert.Equal(t, !tt.shouldFail, pass)
			for _, contains := range tt.expectMsgContains {
				assert.Contains(t, msg, contains)
			}
		})
	}
}

func TestCustomConfigValidationCheck_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupMocks  func(*testing.T) (*filesystem.MockFileSystem, *helm.MockHelmClient, string)
		setParams   []string
		shouldFail  bool
		expectError string
	}{
		{
			name: "filesystem error",
			setupMocks: func(t *testing.T) (*filesystem.MockFileSystem, *helm.MockHelmClient, string) {
				ctrl := gomock.NewController(t)
				mockFS := filesystem.NewMockFileSystem(ctrl)
				mockFS.EXPECT().ReadFile("test.yaml").Return(nil, errors.New("file read error"))
				return mockFS, nil, ""
			},
			setParams:   []string{},
			shouldFail:  true,
			expectError: "cannot read file",
		},
		{
			name: "chart load error",
			setupMocks: func(t *testing.T) (*filesystem.MockFileSystem, *helm.MockHelmClient, string) {
				ctrl := gomock.NewController(t)
				chartPath := t.TempDir()
				// Create Chart.yaml
				err := os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte("name: test\nversion: 1.0.0"), 0644)
				require.NoError(t, err)

				mockClient := helm.NewMockHelmClient(ctrl)
				mockClient.EXPECT().LoadChart(chartPath).Return(nil, errors.New("chart corrupted"))

				mockFS := filesystem.NewMockFileSystem(ctrl)
				mockFS.EXPECT().Stat(chartPath).Return(nil, nil)

				return mockFS, mockClient, chartPath
			},
			setParams: []string{
				"key=value",
			},
			shouldFail:  true,
			expectError: "failed to load chart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFS, mockClient, chartPath := tt.setupMocks(t)

			var setFileParams []string
			if mockFS != nil && tt.name == "filesystem error" {
				setFileParams = []string{"config=test.yaml"}
			}

			check := NewCustomConfigValidationCheck(tt.setParams, setFileParams, chartPath, mockClient)
			if mockFS != nil {
				check.fs = mockFS
			}

			pass, msg, err := check.Run(context.Background())
			require.NoError(t, err)
			assert.Equal(t, !tt.shouldFail, pass)
			if tt.expectError != "" {
				assert.Contains(t, msg, tt.expectError)
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
