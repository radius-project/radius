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
	"io/fs"
	"testing"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCustomConfigValidationCheck_BasicValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		setParams         []string
		setFileParams     []string
		setupFiles        func(t *testing.T, fs filesystem.FileSystem) []string
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
			setupFiles: func(t *testing.T, fs filesystem.FileSystem) []string {
				tmpFile := "/tmp/values.yaml"
				err := fs.WriteFile(tmpFile, []byte("test content"), 0644)
				require.NoError(t, err)
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
			t.Parallel()

			memFS := filesystem.NewMemMapFileSystem()

			setFileParams := tt.setFileParams
			if tt.setupFiles != nil {
				setFileParams = tt.setupFiles(t, memFS)
			}

			check := NewCustomConfigValidationCheck(tt.setParams, setFileParams, "", nil)
			check.fs = memFS
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
	t.Parallel()

	// For chart validation tests, we'll check if real chart exists using default filesystem
	// since this is just for the skip check
	defaultFS := filesystem.NewMemMapFileSystem()
	realChartPath := DefaultChartPath
	if _, err := defaultFS.Stat(realChartPath); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("Radius chart not found at %s, skipping chart validation tests", realChartPath)
	}

	tests := []struct {
		name              string
		setParams         []string
		setFileParams     []string
		setupFiles        func(t *testing.T, fs filesystem.FileSystem) []string
		shouldFail        bool
		expectMsgContains []string
	}{
		{
			name: "valid chart parameters",
			setParams: []string{
				"de.image=ghcr.io/radius-project/deployment-engine:v1.0.0",
				"controller.resources.limits.memory=400Mi",
				"global.prometheus.enabled=false",
				"global.zipkin.url=http://zipkin:9411/api/v2/spans",
			},
			expectMsgContains: []string{
				"validation against Helm chart",
			},
		},
		{
			name: "invalid parameter syntax",
			setParams: []string{
				"de.image[invalid=syntax",
				"controller.resources[invalid].key=test",
			},
			shouldFail: true,
			expectMsgContains: []string{
				"Chart validation failed",
				"failed chart validation",
			},
		},
		{
			name: "valid set-file with chart",
			setupFiles: func(t *testing.T, fs filesystem.FileSystem) []string {
				tmpFile := "/tmp/config.yaml"
				err := fs.WriteFile(tmpFile, []byte("custom-config-content"), 0644)
				require.NoError(t, err)
				return []string{fmt.Sprintf("global.rootCA.cert=%s", tmpFile)}
			},
			expectMsgContains: []string{
				"validation against Helm chart",
			},
		},
		{
			name: "mixed valid parameters with real chart values",
			setParams: []string{
				"de.tag=latest",
				"ucp.image=ghcr.io/radius-project/ucpd:latest",
				"global.azureWorkloadIdentity.enabled=true",
				"controller.logLevel=debug",
			},
			expectMsgContains: []string{
				"All 4 custom configuration parameters passed",
				"validation against Helm chart",
			},
		},
		{
			name: "invalid nested parameter",
			setParams: []string{
				"controller.nonexistent.nested.value=test",
			},
			shouldFail: false, // Helm allows setting non-existent values
			expectMsgContains: []string{
				"validation against Helm chart",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// For chart validation, we need to use the real filesystem
			// since the chart is on disk
			memFS := filesystem.NewMemMapFileSystem()

			// Setup files if needed
			setFileParams := tt.setFileParams
			if tt.setupFiles != nil {
				setFileParams = tt.setupFiles(t, memFS)
			}

			// Use the real chart path
			check := NewCustomConfigValidationCheck(tt.setParams, setFileParams, realChartPath, nil)
			// Use memFS for file parameters but default FS will be used for chart loading
			check.fs = memFS

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
	t.Parallel()

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
				chartPath := "/test/chart"

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
		{
			name: "nonexistent chart path",
			setupMocks: func(t *testing.T) (*filesystem.MockFileSystem, *helm.MockHelmClient, string) {
				ctrl := gomock.NewController(t)
				chartPath := "/nonexistent/chart/path"

				mockFS := filesystem.NewMockFileSystem(ctrl)
				mockFS.EXPECT().Stat(chartPath).Return(nil, fs.ErrNotExist)

				return mockFS, nil, chartPath
			},
			setParams: []string{
				"de.image=test",
			},
			shouldFail:  true,
			expectError: "failed to access chart path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

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
