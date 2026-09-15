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
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
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
			Name:          "valid (basic)",
			Input:         []string{},
			ExpectedValid: true,
		},
		{
			Name:          "valid (advanced)",
			Input:         []string{"--reinstall", "--kubecontext", "foo", "--chart", "test-chart-path", "--set", "foo=bar", "--set", "bar=baz"},
			ExpectedValid: true,
		},
		{
			Name:          "too many args",
			Input:         []string{"blah"},
			ExpectedValid: false,
		},
		{
			Name:          "contour",
			Input:         []string{"--skip-contour-install"},
			ExpectedValid: true,
		},
		{
			Name:          "valid (timeout)",
			Input:         []string{"--timeout", "30m"},
			ExpectedValid: true,
		},
		{
			Name:          "invalid (negative timeout)",
			Input:         []string{"--timeout", "-5m"},
			ExpectedValid: false,
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

// Test_TimeoutFlag covers the `--timeout` flag added so that a slow cluster can be given more
// than the default readiness budget instead of failing the install.
// See https://github.com/radius-project/radius/issues/10236.
func Test_TimeoutFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		expected time.Duration
	}{
		{
			name:     "defaults to the helm default when the flag is omitted",
			args:     []string{},
			expected: helm.DefaultInstallTimeout,
		},
		{
			name:     "parses a longer duration",
			args:     []string{"--timeout", "45m"},
			expected: 45 * time.Minute,
		},
		{
			name:     "parses a shorter duration",
			args:     []string{"--timeout", "90s"},
			expected: 90 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cmd, r := NewCommand(&framework.Impl{Output: &output.MockOutput{}})
			runner, ok := r.(*Runner)
			require.True(t, ok)

			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.ParseFlags(tt.args))

			require.Equal(t, tt.expected, runner.Timeout)
		})
	}
}

// Test_Run_TimeoutPropagation verifies the parsed `--timeout` value reaches the Helm layer via
// ClusterOptions rather than being silently dropped.
func Test_Run_TimeoutPropagation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	helmMock := helm.NewMockInterface(ctrl)
	outputMock := &output.MockOutput{}

	ctx := t.Context()
	runner := &Runner{
		Helm:   helmMock,
		Output: outputMock,

		KubeContext: "test-context",
		Chart:       "test-chart",
		Timeout:     45 * time.Minute,
	}

	helmMock.EXPECT().CheckRadiusInstall("test-context").
		Return(helm.InstallState{}, nil).
		Times(1)

	expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
		Radius: helm.ChartOptions{
			ChartPath: "test-chart",
			Timeout:   45 * time.Minute,
		},
	})
	require.Equal(t, 45*time.Minute, expectedOptions.Radius.Timeout)

	helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
		Return(nil).
		Times(1)

	require.NoError(t, runner.Run(ctx))
}

func Test_Run(t *testing.T) {
	t.Parallel()
	t.Run("Success: Install", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := t.Context()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Chart:       "test-chart",
			Set:         []string{"foo=bar", "bar=baz"},
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{
				ChartPath: "test-chart",
				SetArgs:   []string{"foo=bar", "bar=baz"},
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Already Installed", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := t.Context()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Chart:       "test-chart",
			Set:         []string{"foo=bar", "bar=baz"},
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Found existing Radius installation. Use '--reinstall' to force reinstallation.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Reinstall", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := t.Context()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Chart:       "test-chart",
			Set:         []string{"foo=bar", "bar=baz"},
			Reinstall:   true,
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{
				ChartPath: "test-chart",
				SetArgs:   []string{"foo=bar", "bar=baz"},
				Reinstall: true,
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Reinstalling Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Install with --set and --set-file", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := t.Context()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Chart:       "test-chart",
			Set:         []string{"global.imageRegistry=myregistry.io", "key=value"},
			SetFile:     []string{"global.rootCA.cert=/path/to/cert.crt"},
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{
				ChartPath:   "test-chart",
				SetArgs:     []string{"global.imageRegistry=myregistry.io", "key=value"},
				SetFileArgs: []string{"global.rootCA.cert=/path/to/cert.crt"},
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Install with global.imageTag", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := t.Context()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Set:         []string{"global.imageTag=0.48"},
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{
				SetArgs: []string{"global.imageTag=0.48"},
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Install with both global.imageRegistry and global.imageTag", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := t.Context()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Set:         []string{"global.imageRegistry=myregistry.azurecr.io", "global.imageTag=0.48"},
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{
				SetArgs: []string{"global.imageRegistry=myregistry.azurecr.io", "global.imageTag=0.48"},
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
}
