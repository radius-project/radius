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

package kubernetes

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := framework.NewMockFactory(ctrl)
	mockHelm := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	mockFactory.EXPECT().GetHelmInterface().Return(mockHelm)
	mockFactory.EXPECT().GetOutput().Return(mockOutput)

	cmd, runner := NewCommand(mockFactory)

	require.NotNil(t, cmd)
	require.NotNil(t, runner)
	assert.Equal(t, "kubernetes", cmd.Use)
	assert.Equal(t, "Upgrades Radius on a Kubernetes cluster", cmd.Short)
}

func TestRunner_Validate(t *testing.T) {
	tests := []struct {
		name          string
		skipPreflight bool
		preflightOnly bool
		expectedError string
	}{
		{
			name:          "valid flags",
			skipPreflight: false,
			preflightOnly: false,
			expectedError: "",
		},
		{
			name:          "skip preflight only",
			skipPreflight: true,
			preflightOnly: false,
			expectedError: "",
		},
		{
			name:          "preflight only",
			skipPreflight: false,
			preflightOnly: true,
			expectedError: "",
		},
		{
			name:          "conflicting flags",
			skipPreflight: true,
			preflightOnly: true,
			expectedError: "cannot specify both --skip-preflight and --preflight-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &Runner{
				SkipPreflight: tt.skipPreflight,
				PreflightOnly: tt.preflightOnly,
			}

			err := runner.Validate(nil, nil)

			if tt.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			}
		})
	}
}

func TestRunner_Run(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*helm.MockInterface, *output.MockInterface)
		skipPreflight bool
		preflightOnly bool
		expectError   bool
		errorMessage  string
	}{
		{
			name: "successful upgrade with skipped preflight",
			setupMock: func(mockHelm *helm.MockInterface, mockOutput *output.MockInterface) {
				// Mock CheckRadiusInstall to return installed state
				installState := helm.InstallState{
					RadiusInstalled: true,
					RadiusVersion:   "v0.46.0",
				}
				mockHelm.EXPECT().CheckRadiusInstall("").Return(installState, nil)

				// Mock upgrade
				mockHelm.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "").Return(nil)

				// Mock output logging
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any()).AnyTimes()
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
			},
			skipPreflight: true, // Skip preflight to avoid version validation issues in tests
			preflightOnly: false,
			expectError:   false,
		},
		{
			name: "radius not installed",
			setupMock: func(mockHelm *helm.MockInterface, mockOutput *output.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled: false,
				}
				mockHelm.EXPECT().CheckRadiusInstall("").Return(installState, nil)
			},
			skipPreflight: false,
			preflightOnly: false,
			expectError:   true,
			errorMessage:  "Radius is not currently installed",
		},
		{
			name: "skip preflight and preflight only - should not run",
			setupMock: func(mockHelm *helm.MockInterface, mockOutput *output.MockInterface) {
				// This test case is handled by the Validate function, so no mocks needed
			},
			skipPreflight: true,
			preflightOnly: true,
			expectError:   true,
			errorMessage:  "cannot specify both --skip-preflight and --preflight-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelm := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			tt.setupMock(mockHelm, mockOutput)

			runner := &Runner{
				Helm:          mockHelm,
				Output:        mockOutput,
				SkipPreflight: tt.skipPreflight,
				PreflightOnly: tt.preflightOnly,
			}

			// First run validation
			validateErr := runner.Validate(nil, nil)
			if validateErr != nil {
				// If validation fails, that's our expected error
				if tt.expectError {
					assert.Error(t, validateErr)
					if tt.errorMessage != "" {
						assert.Contains(t, validateErr.Error(), tt.errorMessage)
					}
					return
				} else {
					t.Fatalf("Unexpected validation error: %v", validateErr)
				}
			}

			// If validation passes, run the actual command
			err := runner.Run(context.Background())

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRunner_ResolveTargetVersion(t *testing.T) {
	tests := []struct {
		name           string
		versionFlag    string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "no version specified - uses CLI version",
			versionFlag:    "",
			expectedResult: version.Version(),
			expectError:    false,
		},
		{
			name:           "explicit version specified",
			versionFlag:    "v0.47.0",
			expectedResult: "v0.47.0",
			expectError:    false,
		},
		{
			name:           "latest version - should work with Helm implementation",
			versionFlag:    "latest",
			expectedResult: "v0.47.0", // Will be mocked to return this
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelm := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			if tt.versionFlag == "latest" {
				// Mock the Helm method for getting latest version
				mockHelm.EXPECT().GetLatestRadiusVersion(gomock.Any()).Return("v0.47.0", nil)
				// Mock the log message for latest resolution attempt
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any()).AnyTimes()
			}

			runner := &Runner{
				Helm:    mockHelm,
				Output:  mockOutput,
				Version: tt.versionFlag,
			}

			result, err := runner.resolveTargetVersion()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
