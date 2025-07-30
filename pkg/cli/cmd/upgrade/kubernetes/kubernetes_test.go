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
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	t.Parallel()
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	t.Parallel()
	testcases := []radcli.ValidateInput{
		{
			Name:          "valid - basic",
			Input:         []string{},
			ExpectedValid: true,
		},
		{
			Name: "valid - with all flags",
			Input: []string{
				"--version", "0.48.0",
				"--kubecontext", "my-context",
				"--chart", "/path/to/chart",
				"--set", "key=value",
				"--set-file", "cert=/path/to/cert",
			},
			ExpectedValid: true,
		},
		{
			Name:          "valid - skip preflight",
			Input:         []string{"--skip-preflight"},
			ExpectedValid: true,
		},
		{
			Name:          "valid - preflight only",
			Input:         []string{"--preflight-only"},
			ExpectedValid: true,
		},
		{
			Name:          "invalid - conflicting flags",
			Input:         []string{"--skip-preflight", "--preflight-only"},
			ExpectedValid: false,
		},
		{
			Name:          "invalid - too many args",
			Input:         []string{"extra-arg"},
			ExpectedValid: false,
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func TestNewCommand(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
	tests := []struct {
		name          string
		setupMock     func(*helm.MockInterface, *output.MockInterface)
		version       string
		skipPreflight bool
		preflightOnly bool
		expectError   bool
		errorMessage  string
	}{
		{
			name: "successful upgrade with skipped preflight",
			setupMock: func(mockHelm *helm.MockInterface, mockOutput *output.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled: true,
					RadiusVersion:   "v0.46.0",
				}
				mockHelm.EXPECT().CheckRadiusInstall("").Return(installState, nil)
				mockHelm.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "").Return(nil)
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any()).AnyTimes()
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
			},
			version:       "0.47.0",
			skipPreflight: true,
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
			expectError:  true,
			errorMessage: "the Radius control plane is not currently installed",
		},
		{
			name: "upgrade failure",
			setupMock: func(mockHelm *helm.MockInterface, mockOutput *output.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled: true,
					RadiusVersion:   "v0.46.0",
				}
				mockHelm.EXPECT().CheckRadiusInstall("").Return(installState, nil)
				mockHelm.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "").Return(fmt.Errorf("upgrade failed"))
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any()).AnyTimes()
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
			},
			version:       "0.47.0",
			skipPreflight: true,
			expectError:   true,
			errorMessage:  "failed to upgrade Radius",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelm := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			tt.setupMock(mockHelm, mockOutput)

			runner := &Runner{
				Helm:          mockHelm,
				Output:        mockOutput,
				Version:       tt.version,
				SkipPreflight: tt.skipPreflight,
				PreflightOnly: tt.preflightOnly,
			}

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
	t.Parallel()
	tests := []struct {
		name           string
		versionFlag    string
		expectedResult string
		expectError    bool
		skipCheck      bool
	}{
		{
			name:           "no version specified - uses CLI release version",
			versionFlag:    "",
			expectedResult: version.Release(),
		},
		{
			name:           "explicit version specified",
			versionFlag:    "0.47.0",
			expectedResult: "0.47.0",
		},
		{
			name:           "latest version - should work with Helm implementation",
			versionFlag:    "latest",
			expectedResult: "0.47.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelm := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			if tt.versionFlag == "latest" {
				mockHelm.EXPECT().GetLatestRadiusVersion(gomock.Any()).Return("0.47.0", nil)
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any()).AnyTimes()
			}

			// For edge builds with no version specified, expect latest version resolution
			if tt.versionFlag == "" && version.Release() == "edge" {
				mockOutput.EXPECT().LogInfo("Edge build detected. Upgrading to latest stable version...")
				mockOutput.EXPECT().LogInfo("Resolved to version: %s", "0.47.0")
				mockHelm.EXPECT().GetLatestRadiusVersion(gomock.Any()).Return("0.47.0", nil)
				tt.expectedResult = "0.47.0"
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
				if !tt.skipCheck {
					assert.Equal(t, tt.expectedResult, result)
				}
			}
		})
	}
}

func TestRunner_ResolveTargetVersion_EdgeBuild(t *testing.T) {
	t.Parallel()
	// This test specifically validates the behavior when no version is specified
	// and the CLI is an edge build

	t.Run("edge build resolves to latest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockHelm := helm.NewMockInterface(ctrl)
		mockOutput := output.NewMockInterface(ctrl)

		runner := &Runner{
			Helm:    mockHelm,
			Output:  mockOutput,
			Version: "", // No version specified
		}

		// When the CLI version is "edge" and no version is specified, it should resolve to latest
		if version.Release() == "edge" {
			// Expect log messages about edge build detection
			mockOutput.EXPECT().LogInfo("Edge build detected. Upgrading to latest stable version...")
			mockOutput.EXPECT().LogInfo("Resolved to version: %s", "0.47.0")

			// Expect call to get latest version
			mockHelm.EXPECT().GetLatestRadiusVersion(gomock.Any()).Return("0.47.0", nil)

			result, err := runner.resolveTargetVersion()
			assert.NoError(t, err)
			assert.Equal(t, "0.47.0", result)
		} else {
			// For non-edge builds, it should return the CLI version
			result, err := runner.resolveTargetVersion()
			assert.NoError(t, err)
			assert.Equal(t, version.Release(), result)
		}
	})

	t.Run("edge build fails to get latest", func(t *testing.T) {
		if version.Release() != "edge" {
			t.Skip("This test only runs for edge builds")
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockHelm := helm.NewMockInterface(ctrl)
		mockOutput := output.NewMockInterface(ctrl)

		runner := &Runner{
			Helm:    mockHelm,
			Output:  mockOutput,
			Version: "", // No version specified
		}

		// Expect log message about edge build detection
		mockOutput.EXPECT().LogInfo("Edge build detected. Upgrading to latest stable version...")

		// Expect call to get latest version to fail
		mockHelm.EXPECT().GetLatestRadiusVersion(gomock.Any()).Return("", fmt.Errorf("network error"))

		result, err := runner.resolveTargetVersion()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch latest Radius version")
		assert.Empty(t, result)
	})
}
