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

		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(false, nil).Times(1)
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

		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(true, nil).Times(1)
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
				Params: []any{"radius, contour, cert-manager-csi-driver-spiffe, cert-manager"},
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

		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(false, nil).Times(1)
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

		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(false, nil).Times(1)
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

		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(false, nil).Times(1)
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

		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(false, nil).Times(1)
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
		helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(false, nil).Times(1)
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
			Format: "%s: unable to check for a %s installed by rad: %v",
			Params: []any{logWarningPrefix, "cert-manager", lookupErr},
		})
		require.Contains(t, outputMock.Writes, output.LogOutput{
			Format: "Radius was fully uninstalled. All data has been removed.",
		})
	})
}

func Test_Run_RadiusNotInstalled(t *testing.T) {
	t.Parallel()

	notInstalled := output.LogOutput{Format: "Radius is not installed on the Kubernetes cluster"}
	plan := func(releases string) []any {
		return []any{
			notInstalled,
			output.LogOutput{Format: "Found Radius dependencies installed by rad. This will remove only:"},
			output.LogOutput{Format: "- Helm releases: %s", Params: []any{releases}},
		}
	}
	certManagerPlan := plan("cert-manager")
	finished := output.LogOutput{Format: "Dependency cleanup finished."}
	lookupErr := errors.New("helm get failed")

	tests := []struct {
		name          string
		owned         bool
		ownedErr      error
		spiffeOwned   bool
		spiffeErr     error
		assumeYes     bool
		promptAnswer  string
		wantUninstall bool
		uninstallErr  error
		wantErr       error
		wantWrites    []any
	}{
		{
			name:       "dependencies not owned by rad",
			wantWrites: []any{notInstalled},
		},
		{
			name:      "ownership lookups fail",
			ownedErr:  lookupErr,
			spiffeErr: lookupErr,
			wantWrites: []any{
				notInstalled,
				output.LogOutput{Format: "%s: unable to check for a %s installed by rad: %v", Params: []any{logWarningPrefix, "csi-driver-spiffe", lookupErr}},
				output.LogOutput{Format: "%s: unable to check for a %s installed by rad: %v", Params: []any{logWarningPrefix, "cert-manager", lookupErr}},
			},
		},
		{
			name:          "owned cert-manager removed after confirmation",
			owned:         true,
			promptAnswer:  prompt.ConfirmYes,
			wantUninstall: true,
			wantWrites:    append(append([]any{}, certManagerPlan...), finished),
		},
		{
			name:          "owned csi-driver-spiffe removed after confirmation",
			spiffeOwned:   true,
			promptAnswer:  prompt.ConfirmYes,
			wantUninstall: true,
			wantWrites:    append(plan("cert-manager-csi-driver-spiffe"), finished),
		},
		{
			name:          "owned csi-driver-spiffe and cert-manager removed with --yes",
			owned:         true,
			spiffeOwned:   true,
			assumeYes:     true,
			wantUninstall: true,
			wantWrites: append(plan("cert-manager-csi-driver-spiffe, cert-manager"),
				output.LogOutput{Format: "Skipping confirmation because --yes flag was provided."},
				finished,
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
			helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(tt.spiffeOwned, tt.spiffeErr)
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

// When rad keeps csi-driver-spiffe because Pods still use it, a purge must not delete radius-system, where the driver
// runs, or the kubelet cannot unmount those Pods' certificate volumes. A plain uninstall still succeeds with a warning.
func Test_Run_CSIDriverSPIFFEKept(t *testing.T) {
	t.Parallel()

	keptErr := &helm.CSIDriverSPIFFEKeptError{
		Namespace:  helm.RadiusSystemNamespace,
		DriverName: "spiffe.csi.radapp.io",
		Pods:       []string{"radius-system/ucp-0"},
	}
	tests := []struct {
		name    string
		purge   bool
		err     error
		wantErr []string
		wantLog string
	}{
		{
			name:    "uninstall warns and succeeds",
			err:     keptErr,
			wantLog: "Radius was uninstalled successfully.",
		},
		{
			name:    "wrapped kept error is recognized",
			err:     fmt.Errorf("uninstall: %w", keptErr),
			wantLog: "Radius was uninstalled successfully.",
		},
		{
			name:  "purge stops before deleting namespaces and CRDs",
			purge: true,
			err:   keptErr,
			wantErr: []string{
				"Radius was uninstalled, but rad stopped the purge before deleting namespaces and custom resource definitions because it kept csi-driver-spiffe in namespace radius-system: Pods still mount volumes from it: radius-system/ucp-0.",
				"1. wait until no Pod mounts a volume from CSI driver spiffe.csi.radapp.io\n2. run `rad uninstall kubernetes` to remove csi-driver-spiffe and cert-manager\n",
				`3. kubectl --context "test-context" delete apiservice ` + ucpAPIServiceName + " --ignore-not-found\n",
				`4. kubectl --context "test-context" delete crd ` + strings.Join(radiusCRDs, " ") + " --ignore-not-found\n",
				`5. kubectl --context "test-context" delete namespace radius-system testenv-ns --ignore-not-found`,
			},
		},
		{
			name:  "purge stops when rad could not check for Pods",
			purge: true,
			err:   &helm.CSIDriverSPIFFEKeptError{Namespace: helm.RadiusSystemNamespace, Reason: "could not check whether rad installed it: boom"},
			wantErr: []string{
				"because it kept csi-driver-spiffe in namespace radius-system: could not check whether rad installed it: boom.",
				"1. wait until no Pod mounts a volume from csi-driver-spiffe\n",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			helmMock := helm.NewMockInterface(ctrl)
			k8sMock := kubernetes.NewMockInterface(ctrl)
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			outputMock := &output.MockOutput{}
			ctx := t.Context()

			runner := &Runner{
				Helm:        helmMock,
				Output:      outputMock,
				Kubernetes:  k8sMock,
				Connections: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				KubeContext: "test-context",
				Purge:       tt.purge,
				AssumeYes:   true,
			}

			helmMock.EXPECT().CheckRadiusInstall("test-context").Return(helm.InstallState{RadiusInstalled: true}, nil)
			helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(true, nil)
			helmMock.EXPECT().CertManagerInstalledByRad("test-context").Return(true, nil)
			if tt.purge {
				environment := corerpv20231001.EnvironmentResource{
					ID:   new("/planes/radius/local/resourceGroups/test/providers/Applications.Core/environments/test-env"),
					Name: new("test-env"),
					Properties: &corerpv20231001.EnvironmentProperties{
						Compute: &corerpv20231001.KubernetesCompute{Kind: new("Kubernetes"), Namespace: new("testenv-ns")},
					},
				}
				appManagementClient.EXPECT().ListEnvironmentsAll(ctx).Return([]corerpv20231001.EnvironmentResource{environment}, nil)
				appManagementClient.EXPECT().DeleteEnvironment(ctx, *environment.ID).Return(true, nil)
			}
			// No PerformRadiusCleanup expectation: the strict mock fails the test if the purge deletes anything.
			helmMock.EXPECT().UninstallRadius(ctx, helm.NewDefaultClusterOptions(), "test-context").Return(tt.err)

			err := runner.Run(ctx)
			if len(tt.wantErr) > 0 {
				require.Error(t, err)
				for _, want := range tt.wantErr {
					require.Contains(t, err.Error(), want)
				}
				return
			}
			require.NoError(t, err)
			require.Contains(t, outputMock.Writes, output.LogOutput{
				Format: "%s: %s. Run `rad uninstall kubernetes` again after the Pods that use it are gone to remove them.",
				Params: []any{logWarningPrefix, keptErr.Error()},
			})
			last := outputMock.Writes[len(outputMock.Writes)-1].(output.LogOutput)
			require.True(t, strings.HasPrefix(last.Format, tt.wantLog), last.Format)
		})
	}
}

func Test_Run_RadiusNotInstalled_CSIDriverSPIFFEKept(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	helmMock := helm.NewMockInterface(ctrl)
	outputMock := &output.MockOutput{}
	ctx := t.Context()
	runner := &Runner{Helm: helmMock, Output: outputMock, KubeContext: "test-context", Purge: true, AssumeYes: true}

	keptErr := &helm.CSIDriverSPIFFEKeptError{Namespace: helm.RadiusSystemNamespace, DriverName: "spiffe.csi.radapp.io", Pods: []string{"radius-system/ucp-0"}}
	helmMock.EXPECT().CheckRadiusInstall("test-context").Return(helm.InstallState{}, nil)
	helmMock.EXPECT().CSIDriverSPIFFEInstalledByRad("test-context").Return(true, nil)
	helmMock.EXPECT().CertManagerInstalledByRad("test-context").Return(true, nil)
	helmMock.EXPECT().UninstallCertManager(ctx, helm.NewDefaultClusterOptions(), "test-context").Return(keptErr)

	require.NoError(t, runner.Run(ctx))
	require.Equal(t, output.LogOutput{
		Format: "%s: %s. Run `rad uninstall kubernetes` again after the Pods that use it are gone to remove them.",
		Params: []any{logWarningPrefix, keptErr.Error()},
	}, outputMock.Writes[len(outputMock.Writes)-1])
}
