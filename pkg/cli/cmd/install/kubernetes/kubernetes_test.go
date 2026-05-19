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
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/helm"
	cli_kubernetes "github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/cli/output"
	corerp "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
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
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Parallel()
	// expectDefaultGroupAndEnvCreation sets up the management client mocks needed for the post-install
	// "create default resource group + environment" step (the GET-returns-404 path) and returns the
	// trailing log writes the helper appends so tests can assemble the full expected output slice.
	expectDefaultGroupAndEnvCreation := func(t *testing.T, ctrl *gomock.Controller) (connections.Factory, cli_kubernetes.Interface, []any) {
		t.Helper()
		notFound := &azcore.ResponseError{StatusCode: http.StatusNotFound}
		mgmtMock := clients.NewMockApplicationsManagementClient(ctrl)
		mgmtMock.EXPECT().
			GetResourceGroup(gomock.Any(), "local", "default").
			Return(ucp.ResourceGroupResource{}, notFound).
			Times(1)
		mgmtMock.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _ string, rg *ucp.ResourceGroupResource) error {
				require.NotNil(t, rg)
				require.NotNil(t, rg.Location)
				require.Equal(t, v1.LocationGlobal, *rg.Location)
				return nil
			}).
			Times(1)
		mgmtMock.EXPECT().
			GetEnvironment(gomock.Any(), "default").
			Return(corerp.EnvironmentResource{}, notFound).
			Times(1)
		mgmtMock.EXPECT().
			CreateOrUpdateEnvironment(gomock.Any(), "default", gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, env *corerp.EnvironmentResource) error {
				require.NotNil(t, env)
				require.NotNil(t, env.Location)
				require.Equal(t, v1.LocationGlobal, *env.Location)
				require.NotNil(t, env.Properties)
				k8sCompute, ok := env.Properties.Compute.(*corerp.KubernetesCompute)
				require.True(t, ok, "environment compute must be KubernetesCompute")
				require.NotNil(t, k8sCompute.Namespace)
				require.Equal(t, "default", *k8sCompute.Namespace)
				return nil
			}).
			Times(1)

		// The kube context is no longer resolved by the install command itself: the Runner passes
		// r.KubeContext (possibly empty) straight into the workspace, and the underlying Kubernetes
		// client config treats "" as "use the active kubeconfig context". So the KubernetesInterface
		// mock has no expectations.
		k8sMock := cli_kubernetes.NewMockInterface(ctrl)

		writes := []any{
			output.LogOutput{
				Format: "Creating default resource group %q...",
				Params: []any{"default"},
			},
			output.LogOutput{
				Format: "Creating default environment %q in namespace %q...",
				Params: []any{"default", "default"},
			},
		}
		return &connections.MockFactory{ApplicationsManagementClient: mgmtMock}, k8sMock, writes
	}

	t.Run("Success: Install", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		factory, k8sMock, postInstallWrites := expectDefaultGroupAndEnvCreation(t, ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   factory,
			KubernetesInterface: k8sMock,

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

		expectedWrites := append([]any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}, postInstallWrites...)
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Already Installed", func(t *testing.T) {
		t.Parallel()
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

		// Simulate the typical reinstall scenario: the default resource group and environment
		// already exist (a previous install created them, possibly with user customizations).
		// We expect GETs to succeed and no CreateOrUpdate* calls to be issued.
		mgmtMock := clients.NewMockApplicationsManagementClient(ctrl)
		mgmtMock.EXPECT().
			GetResourceGroup(gomock.Any(), "local", "default").
			Return(ucp.ResourceGroupResource{}, nil).
			Times(1)
		mgmtMock.EXPECT().
			GetEnvironment(gomock.Any(), "default").
			Return(corerp.EnvironmentResource{}, nil).
			Times(1)
		k8sMock := cli_kubernetes.NewMockInterface(ctrl)
		factory := &connections.MockFactory{ApplicationsManagementClient: mgmtMock}

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   factory,
			KubernetesInterface: k8sMock,

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
			output.LogOutput{
				Format: "Default resource group %q already exists; leaving it unchanged.",
				Params: []any{"default"},
			},
			output.LogOutput{
				Format: "Default environment %q already exists; leaving it unchanged.",
				Params: []any{"default"},
			},
		}
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Install with --set and --set-file", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		factory, k8sMock, postInstallWrites := expectDefaultGroupAndEnvCreation(t, ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   factory,
			KubernetesInterface: k8sMock,

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

		expectedWrites := append([]any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}, postInstallWrites...)
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Install with global.imageTag", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		factory, k8sMock, postInstallWrites := expectDefaultGroupAndEnvCreation(t, ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   factory,
			KubernetesInterface: k8sMock,

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

		expectedWrites := append([]any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}, postInstallWrites...)
		require.Equal(t, expectedWrites, outputMock.Writes)
	})
	t.Run("Success: Install with both global.imageRegistry and global.imageTag", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		factory, k8sMock, postInstallWrites := expectDefaultGroupAndEnvCreation(t, ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   factory,
			KubernetesInterface: k8sMock,

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

		expectedWrites := append([]any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}, postInstallWrites...)
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Success: Install with no --kubecontext flag passes empty context through", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}
		factory, k8sMock, postInstallWrites := expectDefaultGroupAndEnvCreation(t, ctrl)

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   factory,
			KubernetesInterface: k8sMock,
			Chart:               "test-chart",
		}

		// An empty kubecontext is correct here and is the established convention across the CLI
		// (see workspaces.MakeFallbackWorkspace): the underlying kube client config interprets ""
		// as "use the active kubeconfig context". rad install kubernetes passes r.KubeContext
		// (possibly empty) straight to Helm and into the post-install workspace.
		helmMock.EXPECT().CheckRadiusInstall("").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{
				ChartPath: "test-chart",
			},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.NoError(t, err)

		expectedWrites := append([]any{
			output.LogOutput{
				Format: "Installing Radius version %s to namespace: %s...",
				Params: []any{"edge", "radius-system"},
			},
		}, postInstallWrites...)
		require.Equal(t, expectedWrites, outputMock.Writes)
	})

	t.Run("Failure: default resource group creation fails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		notFound := &azcore.ResponseError{StatusCode: http.StatusNotFound}
		boom := errors.New("boom")
		mgmtMock := clients.NewMockApplicationsManagementClient(ctrl)
		mgmtMock.EXPECT().
			GetResourceGroup(gomock.Any(), "local", "default").
			Return(ucp.ResourceGroupResource{}, notFound).
			Times(1)
		mgmtMock.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			Return(boom).
			Times(1)
		// GetEnvironment / CreateOrUpdateEnvironment must not be called: the runner should bail out
		// as soon as the resource group create fails.

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: mgmtMock},
			KubernetesInterface: cli_kubernetes.NewMockInterface(ctrl),

			KubeContext: "test-context",
			Chart:       "test-chart",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{ChartPath: "test-chart"},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to create the default resource group")
		require.ErrorIs(t, err, boom)
	})

	t.Run("Failure: default environment creation fails", func(t *testing.T) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		helmMock := helm.NewMockInterface(ctrl)
		outputMock := &output.MockOutput{}

		notFound := &azcore.ResponseError{StatusCode: http.StatusNotFound}
		boom := errors.New("boom")
		mgmtMock := clients.NewMockApplicationsManagementClient(ctrl)
		mgmtMock.EXPECT().
			GetResourceGroup(gomock.Any(), "local", "default").
			Return(ucp.ResourceGroupResource{}, notFound).
			Times(1)
		mgmtMock.EXPECT().
			CreateOrUpdateResourceGroup(gomock.Any(), "local", "default", gomock.Any()).
			Return(nil).
			Times(1)
		mgmtMock.EXPECT().
			GetEnvironment(gomock.Any(), "default").
			Return(corerp.EnvironmentResource{}, notFound).
			Times(1)
		mgmtMock.EXPECT().
			CreateOrUpdateEnvironment(gomock.Any(), "default", gomock.Any()).
			Return(boom).
			Times(1)

		ctx := context.Background()
		runner := &Runner{
			Helm:                helmMock,
			Output:              outputMock,
			ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: mgmtMock},
			KubernetesInterface: cli_kubernetes.NewMockInterface(ctrl),

			KubeContext: "test-context",
			Chart:       "test-chart",
		}

		helmMock.EXPECT().CheckRadiusInstall("test-context").
			Return(helm.InstallState{}, nil).
			Times(1)

		expectedOptions := helm.PopulateDefaultClusterOptions(helm.CLIClusterOptions{
			Radius: helm.ChartOptions{ChartPath: "test-chart"},
		})
		helmMock.EXPECT().InstallRadius(ctx, expectedOptions, "test-context").
			Return(nil).
			Times(1)

		err := runner.Run(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "Failed to create the default environment")
		require.ErrorIs(t, err, boom)
	})
}
