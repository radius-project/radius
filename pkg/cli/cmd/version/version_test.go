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

	// Test default values
	err := runner.Validate(cmd, []string{})
	require.NoError(t, err)
	require.Equal(t, "table", runner.Format)
	require.False(t, runner.CLIOnly)

	// Test with output format set
	cmd = &cobra.Command{}
	cmd.Flags().Bool("cli", false, "")
	cmd.Flags().String("output", "", "")

	err = cmd.Flags().Set("output", "json")
	require.NoError(t, err)

	err = cmd.Flags().Set("cli", "true")
	require.NoError(t, err)

	err = runner.Validate(cmd, []string{})
	require.NoError(t, err)
	require.Equal(t, "json", runner.Format)
	require.True(t, runner.CLIOnly)
}

// TestWriteCliVersionOnly tests the writeCliVersionOnly method
func TestWriteCliVersionOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutput := output.NewMockInterface(ctrl)

	runner := &Runner{
		Output: mockOutput,
	}

	expectedCLIVersion := struct {
		Release string `json:"release"`
		Version string `json:"version"`
		Bicep   string `json:"bicep"`
		Commit  string `json:"commit"`
	}{
		version.Release(),
		version.Version(),
		bicep.Version(),
		version.Commit(),
	}

	mockOutput.EXPECT().WriteFormatted(
		"table",
		gomock.Any(),
		gomock.Any(),
	).Do(func(format string, data any, options output.FormatterOptions) error {
		actualData, ok := data.(struct {
			Release string `json:"release"`
			Version string `json:"version"`
			Bicep   string `json:"bicep"`
			Commit  string `json:"commit"`
		})
		require.True(t, ok)
		require.Equal(t, expectedCLIVersion.Release, actualData.Release)
		require.Equal(t, expectedCLIVersion.Version, actualData.Version)
		require.Equal(t, expectedCLIVersion.Bicep, actualData.Bicep)
		require.Equal(t, expectedCLIVersion.Commit, actualData.Commit)
		return nil
	}).Return(nil)

	err := runner.writeCliVersionOnly("table")
	require.NoError(t, err)
}

// TestWriteVersionInfoSuccess tests the writeVersionInfo method when Radius is installed
func TestWriteVersionInfoSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmInterface := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:   mockHelmInterface,
		Output: mockOutput,
	}

	// Setup Control Plane version check
	installState := helm.InstallState{
		RadiusInstalled: true,
		RadiusVersion:   "v0.45.0",
		DaprVersion:     "1.9.5",
	}

	mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(installState, nil)

	// Setup output expectations
	mockOutput.EXPECT().LogInfo("CLI Version Information:")
	mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)
	mockOutput.EXPECT().LogInfo("\nControl Plane Information:")
	mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)

	err := runner.writeVersionInfo("table")
	require.NoError(t, err)
}

// TestWriteVersionInfoNotInstalled tests the writeVersionInfo method when Radius is not installed
func TestWriteVersionInfoNotInstalled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmInterface := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:   mockHelmInterface,
		Output: mockOutput,
	}

	// Setup Control Plane version check for not installed
	installState := helm.InstallState{
		RadiusInstalled: false,
	}

	mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(installState, nil)

	// Setup output expectations
	mockOutput.EXPECT().LogInfo("CLI Version Information:")
	mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)
	mockOutput.EXPECT().LogInfo("\nControl Plane Information:")
	mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Do(
		func(format string, data interface{}, options output.FormatterOptions) error {
			// Verify that the control plane version is "Not installed"
			cpInfo, ok := data.(struct {
				Version     string `json:"version"`
				DaprVersion string `json:"daprVersion"`
			})
			require.True(t, ok)
			require.Equal(t, "Not installed", cpInfo.Version)
			require.Equal(t, "Not installed", cpInfo.DaprVersion)
			return nil
		}).Return(nil)

	err := runner.writeVersionInfo("table")
	require.NoError(t, err)
}

// TestWriteVersionInfoHelmError tests the writeVersionInfo method when Helm client returns an error
func TestWriteVersionInfoHelmError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmInterface := helm.NewMockInterface(ctrl)
	mockOutput := output.NewMockInterface(ctrl)

	runner := &Runner{
		Helm:   mockHelmInterface,
		Output: mockOutput,
	}

	// Setup Helm client error
	mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(helm.InstallState{}, fmt.Errorf("connection failed"))

	// Setup output expectations
	mockOutput.EXPECT().LogInfo("CLI Version Information:")
	mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)
	mockOutput.EXPECT().LogInfo("Failed to check Radius control plane: %v", fmt.Errorf("connection failed"))
	mockOutput.EXPECT().LogInfo("\nControl Plane Information:")
	mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)

	err := runner.writeVersionInfo("table")
	require.NoError(t, err)
}

// TestRun tests the Run method with various flags
func TestRun(t *testing.T) {
	testCases := []struct {
		name          string
		cliOnly       bool
		expectCLIOnly bool
	}{
		{
			name:          "CLI and Control Plane versions",
			cliOnly:       false,
			expectCLIOnly: false,
		},
		{
			name:          "CLI version only",
			cliOnly:       true,
			expectCLIOnly: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmInterface := helm.NewMockInterface(ctrl)
			mockOutput := output.NewMockInterface(ctrl)

			// Create test command with flag
			cmd := &cobra.Command{}
			cmd.Flags().Bool("cli", tc.cliOnly, "")

			runner := &Runner{
				Helm:    mockHelmInterface,
				Output:  mockOutput,
				CLIOnly: tc.cliOnly,
				Format:  "table",
			}

			if tc.expectCLIOnly {
				mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)
			} else {
				// Setup expected calls for full version info
				mockOutput.EXPECT().LogInfo("CLI Version Information:")
				mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)

				mockHelmInterface.EXPECT().CheckRadiusInstall("").Return(helm.InstallState{
					RadiusInstalled: true,
					RadiusVersion:   "v0.45.0",
					DaprVersion:     "1.9.5",
				}, nil)

				mockOutput.EXPECT().LogInfo("\nControl Plane Information:")
				mockOutput.EXPECT().WriteFormatted("table", gomock.Any(), gomock.Any()).Return(nil)
			}

			err := runner.Run(context.Background())
			require.NoError(t, err)
		})
	}
}
