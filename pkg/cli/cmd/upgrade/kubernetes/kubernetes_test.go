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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/upgrade/preflight"
	"github.com/radius-project/radius/pkg/version"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/pflag"
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
				"--contour-set", "contour.replicaCount=3",
				"--contour-set-file", "envoy.service.annotations.example=/path/to/value.txt",
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
		set           []string
		setFile       []string
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
		{
			name: "successful upgrade with --set and --set-file",
			setupMock: func(mockHelm *helm.MockInterface, mockOutput *output.MockInterface) {
				installState := helm.InstallState{
					RadiusInstalled: true,
					RadiusVersion:   "v0.46.0",
				}
				mockHelm.EXPECT().CheckRadiusInstall("").Return(installState, nil)
				// Verify the correct options are passed
				mockHelm.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "").
					DoAndReturn(func(ctx context.Context, clusterOptions helm.ClusterOptions, kubeContext string) error {
						// Verify SetArgs and SetFileArgs
						expectedSetArgs := []string{"global.imageRegistry=myregistry.io", "key=value"}
						expectedSetFileArgs := []string{"global.rootCA.cert=/path/to/cert.crt"}
						assert.Equal(t, expectedSetArgs, clusterOptions.Radius.SetArgs)
						assert.Equal(t, expectedSetFileArgs, clusterOptions.Radius.SetFileArgs)
						return nil
					})
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any()).AnyTimes()
				mockOutput.EXPECT().LogInfo(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
			},
			version:       "0.47.0",
			skipPreflight: true,
			set:           []string{"global.imageRegistry=myregistry.io", "key=value"},
			setFile:       []string{"global.rootCA.cert=/path/to/cert.crt"},
			expectError:   false,
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
				Set:           tt.set,
				SetFile:       tt.setFile,
			}

			err := runner.Run(t.Context())

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

// Test_Run_ContourFlags verifies that every Contour flag reaches the options passed to UpgradeRadius, so that a
// Contour flag cannot be accepted and then dropped.
func Test_Run_ContourFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		flag string
		args []string
		want func(options *helm.ClusterOptions)
	}{
		{
			flag: "contour-set",
			args: []string{
				"--contour-set", "envoy.service.type=NodePort",
				"--contour-set", "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes}",
			},
			want: func(options *helm.ClusterOptions) {
				options.Contour.SetArgs = []string{"envoy.service.type=NodePort", "contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes}"}
			},
		},
		{
			flag: "contour-set-file",
			args: []string{"--contour-set-file", "envoy.service.annotations.example=/path/to/value.txt"},
			want: func(options *helm.ClusterOptions) {
				options.Contour.SetFileArgs = []string{"envoy.service.annotations.example=/path/to/value.txt"}
			},
		},
	}

	covered := map[string]bool{}
	for _, tt := range tests {
		covered[tt.flag] = true
	}
	cmd, _ := NewCommand(&framework.Impl{Output: &output.MockOutput{}})
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if strings.Contains(flag.Name, "contour") {
			require.True(t, covered[flag.Name], "add a test case for --%s", flag.Name)
		}
	})

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Parallel()

			cmd, r := NewCommand(&framework.Impl{Output: &output.MockOutput{}})
			runner, ok := r.(*Runner)
			require.True(t, ok)
			require.NoError(t, cmd.ParseFlags(append([]string{"--skip-preflight", "--version", "0.47.0"}, tt.args...)))

			helmMock := helm.NewMockInterface(gomock.NewController(t))
			runner.Helm = helmMock

			expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
				Radius: helm.ChartOptions{ChartVersion: "0.47.0"},
			})
			tt.want(&expectedOptions)

			helmMock.EXPECT().CheckRadiusInstall("").
				Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "0.46.0"}, nil).
				Times(1)
			helmMock.EXPECT().UpgradeRadius(gomock.Any(), expectedOptions, "").Return(nil).Times(1)

			require.NoError(t, runner.Run(t.Context()))
		})
	}
}

// Test_Run_PreflightContourCheck verifies that the Contour preflight check uses the same options as the upgrade, so
// that rad upgrade kubernetes --preflight-only reports what the upgrade would do with the Contour release.
func Test_Run_PreflightContourCheck(t *testing.T) {
	t.Parallel()

	for _, preflightOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("preflightOnly=%t", preflightOnly), func(t *testing.T) {
			t.Parallel()

			mockHelm := helm.NewMockInterface(gomock.NewController(t))
			mockHelm.EXPECT().CheckRadiusInstall("my-context").
				Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "0.46.0"}, nil).
				Times(1)
			var checked *helm.ClusterOptions
			mockHelm.EXPECT().CheckContourUpgrade(gomock.Any(), gomock.Any(), "my-context").
				DoAndReturn(func(_ context.Context, options helm.ClusterOptions, _ string) (bool, error) {
					checked = &options
					return true, nil
				}).Times(1)
			var upgraded *helm.ClusterOptions
			if !preflightOnly {
				mockHelm.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "my-context").
					DoAndReturn(func(_ context.Context, options helm.ClusterOptions, _ string) error {
						upgraded = &options
						return nil
					}).Times(1)
			}

			runner := &Runner{
				Helm:           mockHelm,
				Output:         &output.MockOutput{},
				KubeContext:    "my-context",
				Version:        "0.47.0",
				PreflightOnly:  preflightOnly,
				Chart:          "/path/to/chart",
				Set:            []string{"key=value"},
				SetFile:        []string{"global.rootCA.cert=/path/to/cert.crt"},
				ResetValues:    true,
				ContourSet:     []string{"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes}"},
				ContourSetFile: []string{"envoy.service.annotations.example=/path/to/value.txt"},
				runChecks:      runContourCheckOnly(t),
			}
			require.NoError(t, runner.Run(t.Context()))

			require.NotNil(t, checked)
			require.Equal(t, helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
				Radius: helm.ChartOptions{
					ChartPath:    runner.Chart,
					ChartVersion: runner.Version,
					SetArgs:      runner.Set,
					SetFileArgs:  runner.SetFile,
				},
				Contour: helm.ChartOptions{
					SetArgs:     runner.ContourSet,
					SetFileArgs: runner.ContourSetFile,
				},
				ResetValues: true,
			}), *checked)
			if !preflightOnly {
				require.Equal(t, *checked, *upgraded)
			}
		})
	}
}

// Test_Run_FriendlyErrors verifies that rad upgrade kubernetes returns the friendly errors of the Contour checks
// unchanged, so that the CLI shows them as they are instead of as unexpected failures.
func Test_Run_FriendlyErrors(t *testing.T) {
	t.Parallel()

	friendly := clierrors.Message("Radius stopped before changing the cluster because Contour 1.32.0 cannot use the Gateway API CRDs in this cluster.")
	tests := []struct {
		name          string
		skipPreflight bool
		checkErr      error
		upgradeErr    error
		// wantErr is the error that Run returns, or nil when Run returns another error with wantMessage.
		wantErr     error
		wantMessage string
	}{
		{
			name:     "friendly error from the preflight check",
			checkErr: friendly,
			wantErr:  friendly,
		},
		{
			name:        "other error from the preflight check",
			checkErr:    errors.New("failed to list CRDs"),
			wantMessage: "preflight checks failed: pre-flight check 'Contour Upgrade' failed: failed to list CRDs",
		},
		{
			name:          "friendly error from the upgrade",
			skipPreflight: true,
			upgradeErr:    friendly,
			wantErr:       friendly,
		},
		{
			name:          "other error from the upgrade",
			skipPreflight: true,
			upgradeErr:    errors.New("connection refused"),
			wantMessage:   "failed to upgrade Radius: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockHelm := helm.NewMockInterface(gomock.NewController(t))
			mockHelm.EXPECT().CheckRadiusInstall("").
				Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "0.46.0"}, nil).
				Times(1)
			if tt.checkErr != nil {
				mockHelm.EXPECT().CheckContourUpgrade(gomock.Any(), gomock.Any(), "").Return(false, tt.checkErr).Times(1)
				// The upgrade must not run.
			}
			if tt.upgradeErr != nil {
				mockHelm.EXPECT().UpgradeRadius(gomock.Any(), gomock.Any(), "").Return(tt.upgradeErr).Times(1)
			}

			runner := &Runner{
				Helm:          mockHelm,
				Output:        &output.MockOutput{},
				Version:       "0.47.0",
				SkipPreflight: tt.skipPreflight,
				runChecks:     runContourCheckOnly(t),
			}
			err := runner.Run(t.Context())

			if tt.wantErr != nil {
				require.Same(t, tt.wantErr, err)
				require.True(t, clierrors.IsFriendlyError(err))
				return
			}
			require.False(t, clierrors.IsFriendlyError(err))
			require.EqualError(t, err, tt.wantMessage)
		})
	}
}

// runContourCheckOnly runs only the Contour preflight check, through a registry like the command does, because the
// other checks need a cluster.
func runContourCheckOnly(t *testing.T) func(context.Context, []preflight.PreflightCheck) error {
	return func(ctx context.Context, checks []preflight.PreflightCheck) error {
		registry := preflight.NewRegistry(&output.MockOutput{})
		contourChecks := 0
		for _, check := range checks {
			if _, ok := check.(*preflight.ContourUpgradeCheck); ok {
				registry.AddCheck(check)
				contourChecks++
			}
		}
		require.Equal(t, 1, contourChecks, "the preflight checks must include the Contour upgrade check")

		_, err := registry.RunChecks(ctx)
		return err
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
