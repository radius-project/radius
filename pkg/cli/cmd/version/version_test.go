package version

import (
	"context"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/version"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestNewCommand tests that the command is created correctly with the right flags and configuration
func TestNewCommand(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFactory := framework.NewMockFactory(ctrl)
	mockHelmInterface := helm.NewMockInterface(ctrl)
	mockOutput := &output.MockOutput{}

	mockFactory.EXPECT().GetHelmInterface().Return(mockHelmInterface)
	mockFactory.EXPECT().GetOutput().Return(mockOutput)

	cmd, runner := NewCommand(mockFactory)

	// Verify command metadata
	require.Equal(t, "version", cmd.Use)
	require.Equal(t, "Prints the versions of the rad CLI and the Control Plane", cmd.Short)

	// Verify flags
	cliOnly, err := cmd.Flags().GetBool("cli")
	require.NoError(t, err)
	require.False(t, cliOnly)

	// Verify runner was created properly
	versionRunner, ok := runner.(*Runner)
	require.True(t, ok)
	require.Equal(t, mockHelmInterface, versionRunner.Helm)
	require.Equal(t, mockOutput, versionRunner.Output)
}

// TestValidate tests the Validate function behavior
func TestValidate(t *testing.T) {
	testCases := []struct {
		name            string
		outputFlag      string
		cliFlag         string
		expectedFormat  string
		expectedCLIOnly bool
	}{
		{
			name:            "Default values",
			outputFlag:      "",
			cliFlag:         "false",
			expectedFormat:  "table",
			expectedCLIOnly: false,
		},
		{
			name:            "JSON output and CLI only",
			outputFlag:      "json",
			cliFlag:         "true",
			expectedFormat:  "json",
			expectedCLIOnly: true,
		},
		{
			name:            "YAML output",
			outputFlag:      "yaml",
			cliFlag:         "false",
			expectedFormat:  "yaml",
			expectedCLIOnly: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmInterface := helm.NewMockInterface(ctrl)
			mockOutput := &output.MockOutput{}

			runner := &Runner{
				Helm:   mockHelmInterface,
				Output: mockOutput,
			}

			cmd := &cobra.Command{}
			cmd.Flags().Bool("cli", false, "")
			cmd.Flags().String("output", "", "")

			if tc.outputFlag != "" {
				err := cmd.Flags().Set("output", tc.outputFlag)
				require.NoError(t, err)
			}

			err := cmd.Flags().Set("cli", tc.cliFlag)
			require.NoError(t, err)

			err = runner.Validate(cmd, []string{})
			require.NoError(t, err)
			require.Equal(t, tc.expectedFormat, runner.Format)
			require.Equal(t, tc.expectedCLIOnly, runner.CLIOnly)
		})
	}
}

// TestWriteCliVersionOnly tests the writeCliVersionOnly method
func TestWriteCliVersionOnly(t *testing.T) {
	testCases := []struct {
		name         string
		format       string
		expectedInfo CLIVersionInfo
	}{
		{
			name:   "Table format",
			format: "table",
			expectedInfo: CLIVersionInfo{
				Release: version.Release(),
				Version: version.Version(),
				Bicep:   bicep.Version(),
				Commit:  version.Commit(),
			},
		},
		{
			name:   "JSON format",
			format: "json",
			expectedInfo: CLIVersionInfo{
				Release: version.Release(),
				Version: version.Version(),
				Bicep:   bicep.Version(),
				Commit:  version.Commit(),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOutput := output.NewMockInterface(ctrl)
			runner := &Runner{
				Output: mockOutput,
			}

			mockOutput.EXPECT().WriteFormatted(
				tc.format,
				gomock.Any(),
				gomock.Any(),
			).Do(func(format string, data any, options output.FormatterOptions) error {
				actualData, ok := data.(CLIVersionInfo)
				require.True(t, ok)
				require.Equal(t, tc.expectedInfo.Release, actualData.Release)
				require.Equal(t, tc.expectedInfo.Version, actualData.Version)
				require.Equal(t, tc.expectedInfo.Bicep, actualData.Bicep)
				require.Equal(t, tc.expectedInfo.Commit, actualData.Commit)
				return nil
			}).Return(nil)

			err := runner.writeCliVersionOnly(tc.format)
			require.NoError(t, err)
		})
	}
}

// TestWriteVersionInfo tests the writeVersionInfo method with different scenarios
func TestWriteVersionInfo(t *testing.T) {
	testCases := []struct {
		name            string
		format          string
		installState    helm.InstallState
		helmError       error
		expectedStatus  string
		expectedVersion string
		showHeaders     bool
	}{
		{
			name:   "Radius installed - table format",
			format: "table",
			installState: helm.InstallState{
				RadiusInstalled: true,
				RadiusVersion:   "v0.45.0",
			},
			helmError:       nil,
			expectedStatus:  "Installed",
			expectedVersion: "v0.45.0",
			showHeaders:     true,
		},
		{
			name:   "Radius not installed - table format",
			format: "table",
			installState: helm.InstallState{
				RadiusInstalled: false,
			},
			helmError:       nil,
			expectedStatus:  "Not installed",
			expectedVersion: "Not installed",
			showHeaders:     true,
		},
		{
			name:            "Connection error - table format",
			format:          "table",
			installState:    helm.InstallState{},
			helmError:       fmt.Errorf("connection failed"),
			expectedStatus:  "Not connected",
			expectedVersion: "Not installed",
			showHeaders:     true,
		},
		{
			name:   "Radius installed - JSON format",
			format: "json",
			installState: helm.InstallState{
				RadiusInstalled: true,
				RadiusVersion:   "v0.45.0",
			},
			helmError:       nil,
			expectedStatus:  "Installed",
			expectedVersion: "v0.45.0",
			showHeaders:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmInterface := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			runner := &Runner{
				Helm:   mockHelmInterface,
				Output: mockOutput,
				Format: tc.format,
			}

			mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(tc.installState, tc.helmError)

			// Only expect headers for formats that show them
			if tc.showHeaders {
				mockOutput.EXPECT().LogInfo("CLI Version Information:")
			}

			mockOutput.EXPECT().WriteFormatted(tc.format, gomock.Any(), gomock.Any()).Return(nil)

			if tc.showHeaders {
				mockOutput.EXPECT().LogInfo("\nControl Plane Information:")
			}

			mockOutput.EXPECT().WriteFormatted(tc.format, gomock.Any(), gomock.Any()).Do(
				func(format string, data any, options output.FormatterOptions) error {
					controlPlane, ok := data.(ControlPlaneVersionInfo)
					require.True(t, ok)
					require.Equal(t, tc.expectedStatus, controlPlane.Status)
					require.Equal(t, tc.expectedVersion, controlPlane.Version)
					return nil
				}).Return(nil)

			err := runner.writeVersionInfo(tc.format)
			require.NoError(t, err)
		})
	}
}

// TestRun tests the Run method with various flags
func TestRun(t *testing.T) {
	testCases := []struct {
		name          string
		cliOnly       bool
		format        string
		expectCLIOnly bool
	}{
		{
			name:          "CLI and Control Plane versions - table format",
			cliOnly:       false,
			format:        "table",
			expectCLIOnly: false,
		},
		{
			name:          "CLI version only - table format",
			cliOnly:       true,
			format:        "table",
			expectCLIOnly: true,
		},
		{
			name:          "CLI and Control Plane versions - JSON format",
			cliOnly:       false,
			format:        "json",
			expectCLIOnly: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmInterface := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			runner := &Runner{
				Helm:    mockHelmInterface,
				Output:  mockOutput,
				CLIOnly: tc.cliOnly,
				Format:  tc.format,
			}

			if tc.expectCLIOnly {
				mockOutput.EXPECT().WriteFormatted(tc.format, gomock.Any(), gomock.Any()).Return(nil)
			} else {
				// Setup expected calls for full version info
				if tc.format != "json" && tc.format != "yaml" {
					mockOutput.EXPECT().LogInfo("CLI Version Information:")
				}
				mockOutput.EXPECT().WriteFormatted(tc.format, gomock.Any(), gomock.Any()).Return(nil)

				mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(helm.InstallState{
					RadiusInstalled: true,
					RadiusVersion:   "v0.45.0",
				}, nil)

				if tc.format != "json" && tc.format != "yaml" {
					mockOutput.EXPECT().LogInfo("\nControl Plane Information:")
				}
				mockOutput.EXPECT().WriteFormatted(tc.format, gomock.Any(), gomock.Any()).Return(nil)
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})
	}
}

// TestGetControlPlaneVersionInfo tests the getControlPlaneVersionInfo method
func TestGetControlPlaneVersionInfo(t *testing.T) {
	testCases := []struct {
		name            string
		installState    helm.InstallState
		helmError       error
		expectedStatus  string
		expectedVersion string
	}{
		{
			name: "Radius installed",
			installState: helm.InstallState{
				RadiusInstalled: true,
				RadiusVersion:   "v0.45.0",
			},
			helmError:       nil,
			expectedStatus:  "Installed",
			expectedVersion: "v0.45.0",
		},
		{
			name: "Radius not installed",
			installState: helm.InstallState{
				RadiusInstalled: false,
			},
			helmError:       nil,
			expectedStatus:  "Not installed",
			expectedVersion: "Not installed",
		},
		{
			name:            "Connection error",
			installState:    helm.InstallState{},
			helmError:       fmt.Errorf("connection failed"),
			expectedStatus:  "Not connected",
			expectedVersion: "Not installed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmInterface := helm.NewMockInterface(ctrl)

			runner := &Runner{
				Helm: mockHelmInterface,
			}

			mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(tc.installState, tc.helmError)

			cpInfo := runner.getControlPlaneVersionInfo()

			require.Equal(t, tc.expectedStatus, cpInfo.Status)
			require.Equal(t, tc.expectedVersion, cpInfo.Version)
		})
	}
}

// TestGetControlPlaneFormatterOptions tests the formatter options for control plane info
func TestGetControlPlaneFormatterOptions(t *testing.T) {
	expectedColumns := []struct {
		heading  string
		jsonPath string
	}{
		{
			heading:  "STATUS",
			jsonPath: "{ .Status }",
		},
		{
			heading:  "VERSION",
			jsonPath: "{ .Version }",
		},
	}

	options := getControlPlaneFormatterOptions()
	require.Equal(t, len(expectedColumns), len(options.Columns))

	for i, expected := range expectedColumns {
		require.Equal(t, expected.heading, options.Columns[i].Heading)
		require.Equal(t, expected.jsonPath, options.Columns[i].JSONPath)
	}
}
