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

	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
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
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: Install", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := context.Background()
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
			Radius: helm.RadiusOptions{
				ChartPath: "test-chart",
				SetArgs:   []string{"foo=bar", "bar=baz"},
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(true, nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []interface{}{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Already Installed", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := context.Background()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Chart:       "test-chart",
			Set:         []string{"foo=bar", "bar=baz"},
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{Installed: true, Version: "test-version"}, nil).
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
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := context.Background()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
			Chart:       "test-chart",
			Set:         []string{"foo=bar", "bar=baz"},
			Reinstall:   true,
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{Installed: true, Version: "test-version"}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.RadiusOptions{
				ChartPath: "test-chart",
				SetArgs:   []string{"foo=bar", "bar=baz"},
				Reinstall: true,
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(true, nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Reinstalling Radius version %s to namespace: %s...",
				Params: []interface{}{"edge", "radius-system"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
}
