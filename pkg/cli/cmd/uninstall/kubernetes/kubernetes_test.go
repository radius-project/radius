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

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/to"

	"github.com/radius-project/radius/pkg/cli/helm"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20231001 "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type errorApplicationsFactory struct {
	*connections.MockFactory
	err error
}

func (f *errorApplicationsFactory) CreateApplicationsManagementClient(ctx context.Context, workspace workspaces.Workspace) (clients.ApplicationsManagementClient, error) {
	return nil, f.err
}

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
		promptMock := prompt.NewMockInterface(ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:        helmMock,
			Output:      outputMock,
			Connections: &connections.MockFactory{},
			Prompter:    promptMock,

			KubeContext: "test-context",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)

		promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "About to uninstall Radius. This will remove:",
			},
			output.LogOutput{
				Format: "- Helm releases: %s",
				Params: []any{"radius"},
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
		promptMock := prompt.NewMockInterface(ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:        helmMock,
			Output:      outputMock,
			Connections: &connections.MockFactory{},
			Prompter:    promptMock,

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
	t.Run("Success: Installed -> Uninstalled -> Purge", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		k8sMock := kubernetes.NewMockInterface(ctrl)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		promptMock := prompt.NewMockInterface(ctrl)
		connFactory := &connections.MockFactory{ApplicationsManagementClient: appManagementClient}

		ctx := context.Background()
		runner := &Runner{
			Helm:        helmMock,
			Output:      outputMock,
			Kubernetes:  k8sMock,
			Connections: connFactory,
			Prompter:    promptMock,

			KubeContext: "test-context",
			Purge:       true,
		}

		envID := "/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/test-env"
		environment := corerpv20231001.EnvironmentResource{
			ID:   to.Ptr(envID),
			Name: to.Ptr("test-env"),
			Properties: &corerpv20231001.EnvironmentProperties{
				Compute: &corerpv20231001.KubernetesCompute{
					Kind:      to.Ptr("Kubernetes"),
					Namespace: to.Ptr("testenv-ns"),
				},
			},
		}

		appManagementClient.EXPECT().ListEnvironmentsAll(ctx).
			Return([]corerpv20231001.EnvironmentResource{environment}, nil).
			Times(1)

		appManagementClient.EXPECT().DeleteEnvironment(ctx, envID).
			Return(true, nil).
			Times(1)

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version", ContourInstalled: true}, nil).
			Times(1)

		promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		expectedCleanup := kubernetes.CleanupPlan{
			Namespaces:  []string{helm.RadiusSystemNamespace, daprSystemNamespace},
			APIServices: []string{ucpAPIServiceName},
			CRDs:        radiusCRDs,
		}
		k8sMock.EXPECT().PerformRadiusCleanup(ctx, "test-context", expectedCleanup).Return(nil).Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "About to uninstall Radius. This will remove:",
			},
			output.LogOutput{
				Format: "- Helm releases: %s",
				Params: []any{"radius, contour"},
			},
			output.LogOutput{
				Format: "- Radius environments:",
			},
			output.LogOutput{
				Format: "  • %s (namespace %s)",
				Params: []any{envID, "testenv-ns"},
			},
			output.LogOutput{
				Format: "- Kubernetes namespaces: %s",
				Params: []any{"radius-system, dapr-system"},
			},
			output.LogOutput{
				Format: "- Kubernetes API services: %s",
				Params: []any{ucpAPIServiceName},
			},
			output.LogOutput{
				Format: "- Kubernetes custom resource definitions: %s",
				Params: []any{strings.Join(radiusCRDs, ", ")},
			},
			output.LogOutput{
				Format: "Deleting environment %s",
				Params: []any{envID},
			},
			output.LogOutput{
				Format: "Removing APIService %s",
				Params: []any{ucpAPIServiceName},
			},
			output.LogOutput{
				Format: "Removing Radius custom resource definitions",
			},
			output.LogOutput{
				Format: "Deleting namespace %s",
				Params: []any{helm.RadiusSystemNamespace},
			},
			output.LogOutput{
				Format: "Deleting namespace %s",
				Params: []any{daprSystemNamespace},
			},
			output.LogOutput{
				Format: "Radius was fully uninstalled. All data has been removed.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Cancel: User declines uninstall", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		promptMock := prompt.NewMockInterface(ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:        helmMock,
			Output:      outputMock,
			Connections: &connections.MockFactory{},
			Prompter:    promptMock,

			KubeContext: "test-context",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)

		promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).
			Return(prompt.ConfirmNo, nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "About to uninstall Radius. This will remove:",
			},
			output.LogOutput{
				Format: "- Helm releases: %s",
				Params: []any{"radius"},
			},
			output.LogOutput{
				Format: "Uninstall cancelled.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Success: Installed -> Uninstalled (AssumeYes)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		ctx := context.Background()
		runner := &Runner{
			Helm:       helmMock,
			Output:     outputMock,
			Kubernetes: kubernetes.NewMockInterface(ctrl),
			AssumeYes:  true,

			KubeContext: "test-context",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := []any{
			output.LogOutput{
				Format: "About to uninstall Radius. This will remove:",
			},
			output.LogOutput{
				Format: "- Helm releases: %s",
				Params: []any{"radius"},
			},
			output.LogOutput{
				Format: "Skipping confirmation because --yes flag was provided.",
			},
			output.LogOutput{
				Format: "Radius was uninstalled successfully. Any existing data will be retained for future installations. Local configuration is also retained. Use the `rad workspace` command if updates are needed to your configuration.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Warning: Purge continues when environment discovery fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		k8sMock := kubernetes.NewMockInterface(ctrl)
		envErr := errors.New("failed to connect")
		factory := &errorApplicationsFactory{MockFactory: &connections.MockFactory{}, err: envErr}

		ctx := context.Background()
		runner := &Runner{
			Helm:        helmMock,
			Output:      outputMock,
			Kubernetes:  k8sMock,
			Connections: factory,
			AssumeYes:   true,

			KubeContext: "test-context",
			Purge:       true,
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version", ContourInstalled: true}, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		expectedCleanup := kubernetes.CleanupPlan{
			Namespaces:  []string{helm.RadiusSystemNamespace, daprSystemNamespace},
			APIServices: []string{ucpAPIServiceName},
			CRDs:        radiusCRDs,
		}
		k8sMock.EXPECT().PerformRadiusCleanup(ctx, "test-context", expectedCleanup).Return(nil).Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWarningErr := fmt.Errorf("creating applications client: %w", envErr)
		expectedWrites := []any{
			output.LogOutput{
				Format: "%s: unable to enumerate Radius environments via Radius APIs: %v",
				Params: []any{logWarningPrefix, expectedWarningErr},
			},
			output.LogOutput{
				Format: "About to uninstall Radius. This will remove:",
			},
			output.LogOutput{
				Format: "- Helm releases: %s",
				Params: []any{"radius, contour"},
			},
			output.LogOutput{
				Format: "- Radius environments: unable to determine (Radius management APIs unreachable)",
			},
			output.LogOutput{
				Format: "- Kubernetes namespaces: %s",
				Params: []any{"radius-system, dapr-system"},
			},
			output.LogOutput{
				Format: "- Kubernetes API services: %s",
				Params: []any{ucpAPIServiceName},
			},
			output.LogOutput{
				Format: "- Kubernetes custom resource definitions: %s",
				Params: []any{strings.Join(radiusCRDs, ", ")},
			},
			output.LogOutput{
				Format: "Skipping confirmation because --yes flag was provided.",
			},
			output.LogOutput{
				Format: "%s: skipping Radius environment deletion because the Radius management APIs could not be reached",
				Params: []any{logWarningPrefix},
			},
			output.LogOutput{
				Format: "Removing APIService %s",
				Params: []any{ucpAPIServiceName},
			},
			output.LogOutput{
				Format: "Removing Radius custom resource definitions",
			},
			output.LogOutput{
				Format: "Deleting namespace %s",
				Params: []any{helm.RadiusSystemNamespace},
			},
			output.LogOutput{
				Format: "Deleting namespace %s",
				Params: []any{daprSystemNamespace},
			},
			output.LogOutput{
				Format: "Radius was fully uninstalled. All data has been removed.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
}
