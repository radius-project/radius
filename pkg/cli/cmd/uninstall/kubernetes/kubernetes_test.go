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
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"testing"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	testcases := []radcli.ValidateInput{
		{
			Name:          "valid",
			Input:         []string{},
			ExpectedValid: true,
		},
		{
			Name:          "valid (advanced)",
			Input:         []string{"--kubecontext", "test-context"},
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
	t.Run("Success: Installed -> Uninstalled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := context.Background()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{Installed: true, Version: "test-version"}, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Uninstalling Radius...",
			},
			output.LogOutput{
				Format: "Radius was uninstalled successfully. Any existing data will be retained for future installations. Local configuration is also retained. Use the `rad workspace` command if updates are needed to your configuration.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Success: Not Installed -> Uninstalled", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := context.Background()
		runner := &Runner{
			Helm:   helmMock,
			Output: outputMock,

			KubeContext: "test-context",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Radius is not installed on the Kubernetes cluster",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Installed -> Uninstalled -> Purge)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		k8sMock := kubernetes.NewMockInterface(ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:       helmMock,
			Output:     outputMock,
			Kubernetes: k8sMock,

			KubeContext: "test-context",
			Purge:       true,
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{Installed: true, Version: "test-version"}, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, "test-context").
			Return(nil).
			Times(1)

		k8sMock.EXPECT().DeleteNamespace("test-context").Return(nil).Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "Uninstalling Radius...",
			},
			output.LogOutput{
				Format: "Deleting namespace %s",
				Params: []any{helm.RadiusSystemNamespace},
			},
			output.LogOutput{
				Format: "Radius was fully uninstalled. Any existing data have been removed.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
}
