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

		ctx := t.Context()
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

		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(false, nil).
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

	t.Run("Success: Installed -> Uninstalled -> Purge", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		k8sMock := kubernetes.NewMockInterface(ctrl)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		promptMock := prompt.NewMockInterface(ctrl)
		connFactory := &connections.MockFactory{ApplicationsManagementClient: appManagementClient}

		ctx := t.Context()
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
			ID:   new(envID),
			Name: new("test-env"),
			Properties: &corerpv20231001.EnvironmentProperties{
				Compute: &corerpv20231001.KubernetesCompute{
					Kind:      new("Kubernetes"),
					Namespace: new("testenv-ns"),
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

		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(true, nil).
			Times(1)

		promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		expectedCleanup := kubernetes.CleanupPlan{
			Namespaces:  []string{helm.RadiusSystemNamespace, "testenv-ns"},
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
				Params: []any{"radius, contour, cert-manager"},
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
				Params: []any{"radius-system, testenv-ns"},
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
				Params: []any{"testenv-ns"},
			},
			output.LogOutput{
				Format: "Waiting for resource deletions to finish...",
			},
			output.LogOutput{
				Format: "Radius was fully uninstalled. All data has been removed.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Warning: Skip deleting default namespace", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		k8sMock := kubernetes.NewMockInterface(ctrl)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		promptMock := prompt.NewMockInterface(ctrl)
		connFactory := &connections.MockFactory{ApplicationsManagementClient: appManagementClient}

		ctx := t.Context()
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
			ID:   new(envID),
			Name: new("test-env"),
			Properties: &corerpv20231001.EnvironmentProperties{
				Compute: &corerpv20231001.KubernetesCompute{
					Kind:      new("Kubernetes"),
					Namespace: new("default"),
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
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)

		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(false, nil).
			Times(1)

		promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).
			Return(prompt.ConfirmYes, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		expectedCleanup := kubernetes.CleanupPlan{
			Namespaces:  []string{helm.RadiusSystemNamespace},
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
				Params: []any{"radius"},
			},
			output.LogOutput{
				Format: "- Radius environments:",
			},
			output.LogOutput{
				Format: "  • %s (namespace %s)",
				Params: []any{envID, "default"},
			},
			output.LogOutput{
				Format: "- Kubernetes namespaces: %s",
				Params: []any{"radius-system"},
			},
			output.LogOutput{
				Format: "- Kubernetes namespaces (skipped): %s",
				Params: []any{"default"},
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
				Format: "Skipping deletion of namespace %s because Kubernetes does not allow deletion",
				Params: []any{"default"},
			},
			output.LogOutput{
				Format: "Waiting for resource deletions to finish...",
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

		ctx := t.Context()
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

		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(false, nil).
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

		ctx := t.Context()
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

		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(false, nil).
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

		ctx := t.Context()
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

		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(false, nil).
			Times(1)

		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		expectedCleanup := kubernetes.CleanupPlan{
			Namespaces:  []string{helm.RadiusSystemNamespace},
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
				Params: []any{"radius-system"},
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
				Format: "Waiting for resource deletions to finish...",
			},
			output.LogOutput{
				Format: "Radius was fully uninstalled. All data has been removed.",
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Warning: Purge continues when cert-manager ownership lookup fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		k8sMock := kubernetes.NewMockInterface(ctrl)
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		connFactory := &connections.MockFactory{ApplicationsManagementClient: appManagementClient}
		lookupErr := errors.New("helm get failed")

		ctx := t.Context()
		runner := &Runner{
			Helm:        helmMock,
			Output:      outputMock,
			Kubernetes:  k8sMock,
			Connections: connFactory,
			AssumeYes:   true,

			KubeContext: "test-context",
			Purge:       true,
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{RadiusInstalled: true, RadiusVersion: "test-version"}, nil).
			Times(1)
		helmMock.EXPECT().CertManagerInstalledByRad("test-context").
			Return(false, lookupErr).
			Times(1)
		appManagementClient.EXPECT().ListEnvironmentsAll(ctx).
			Return([]corerpv20231001.EnvironmentResource{}, nil).
			Times(1)
		helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").
			Return(nil).
			Times(1)

		expectedCleanup := kubernetes.CleanupPlan{
			Namespaces:  []string{helm.RadiusSystemNamespace},
			APIServices: []string{ucpAPIServiceName},
			CRDs:        radiusCRDs,
		}
		k8sMock.EXPECT().PerformRadiusCleanup(ctx, "test-context", expectedCleanup).Return(nil).Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)
		require.Contains(t, outputMock.Writes, output.LogOutput{
			Format: "%s: unable to check for a cert-manager installed by rad: %v",
			Params: []any{logWarningPrefix, lookupErr},
		})
		require.Contains(t, outputMock.Writes, output.LogOutput{
			Format: "Radius was fully uninstalled. All data has been removed.",
		})
	})
}

func Test_Run_RadiusNotInstalled(t *testing.T) {
	t.Parallel()

	notInstalled := output.LogOutput{Format: "Radius is not installed on the Kubernetes cluster"}
	certManagerPlan := []any{
		notInstalled,
		output.LogOutput{Format: "Found a cert-manager installed by rad. This will remove only:"},
		output.LogOutput{Format: "- Helm releases: %s", Params: []any{"cert-manager"}},
	}
	lookupErr := errors.New("helm get failed")

	tests := []struct {
		name          string
		owned         bool
		ownedErr      error
		assumeYes     bool
		promptAnswer  string
		wantUninstall bool
		uninstallErr  error
		wantErr       error
		wantWrites    []any
	}{
		{
			name:       "cert-manager not owned by rad",
			wantWrites: []any{notInstalled},
		},
		{
			name:     "cert-manager ownership lookup fails",
			ownedErr: lookupErr,
			wantWrites: []any{
				notInstalled,
				output.LogOutput{Format: "%s: unable to check for a cert-manager installed by rad: %v", Params: []any{logWarningPrefix, lookupErr}},
			},
		},
		{
			name:          "owned cert-manager removed after confirmation",
			owned:         true,
			promptAnswer:  prompt.ConfirmYes,
			wantUninstall: true,
			wantWrites:    append(append([]any{}, certManagerPlan...), output.LogOutput{Format: "cert-manager cleanup finished."}),
		},
		{
			name:          "owned cert-manager removed with --yes",
			owned:         true,
			assumeYes:     true,
			wantUninstall: true,
			wantWrites: append(append([]any{}, certManagerPlan...),
				output.LogOutput{Format: "Skipping confirmation because --yes flag was provided."},
				output.LogOutput{Format: "cert-manager cleanup finished."},
			),
		},
		{
			name:         "owned cert-manager kept when user declines",
			owned:        true,
			promptAnswer: prompt.ConfirmNo,
			wantWrites:   append(append([]any{}, certManagerPlan...), output.LogOutput{Format: "Uninstall cancelled."}),
		},
		{
			// The ownership re-check and other-workload guard live in helm.Interface.UninstallCertManager.
			name:          "owned cert-manager uninstall error is returned",
			owned:         true,
			assumeYes:     true,
			wantUninstall: true,
			uninstallErr:  lookupErr,
			wantErr:       lookupErr,
			wantWrites: append(append([]any{}, certManagerPlan...),
				output.LogOutput{Format: "Skipping confirmation because --yes flag was provided."},
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			helmMock := helm.NewMockInterface(ctrl)
			promptMock := prompt.NewMockInterface(ctrl)
			outputMock := &output.MockOutput{}
			ctx := t.Context()

			// Strict mocks with no Kubernetes, UninstallRadius, or environment expectations prove that
			// this path never touches Radius, Contour, or Radius data, even with --purge.
			runner := &Runner{
				Helm:        helmMock,
				Output:      outputMock,
				Kubernetes:  kubernetes.NewMockInterface(ctrl),
				Connections: &connections.MockFactory{},
				Prompter:    promptMock,
				KubeContext: "test-context",
				Purge:       true,
				AssumeYes:   tt.assumeYes,
			}

			helmMock.EXPECT().CheckRadiusInstall("test-context").Return(helm.InstallState{}, nil)
			helmMock.EXPECT().CertManagerInstalledByRad("test-context").Return(tt.owned, tt.ownedErr)
			if tt.promptAnswer != "" {
				promptMock.EXPECT().GetListInput([]string{prompt.ConfirmNo, prompt.ConfirmYes}, gomock.Any()).
					Return(tt.promptAnswer, nil)
			}
			if tt.wantUninstall {
				helmMock.EXPECT().UninstallCertManager(ctx, helm.NewDefaultClusterOptions(), "test-context").
					Return(tt.uninstallErr)
			}

			err := runner.Run(ctx)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.wantWrites, outputMock.Writes)
		})
	}
}
