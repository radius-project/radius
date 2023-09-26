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

package run

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clients"
	deploycmd "github.com/radius-project/radius/pkg/cli/cmd/deploy"
	"github.com/radius-project/radius/pkg/cli/config"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/deploy"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/kubernetes/logstream"
	"github.com/radius-project/radius/pkg/cli/kubernetes/portforward"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/testcontext"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	// NOTE: most of the logic of this command is shared with the `rad deploy` command.
	// We're using a few of the same tests here as a smoke test, but the bulk of the testing
	// is part of the `rad deploy` tests.
	//
	// We should revisit the test strategy if the code paths deviate sigificantly.
	testcases := []radcli.ValidateInput{
		{
			Name:          "rad run - valid with app and env",
			Input:         []string{"app.bicep", "-e", "prod", "-a", "my-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - app set by directory config",
			Input:         []string{"app.bicep", "-e", "prod"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
				DirectoryConfig: &config.DirectoryConfig{
					Workspace: config.DirectoryWorkspaceConfig{
						Application: "my-app",
					},
				},
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), "prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - app is required invalid",
			Input:         []string{"app.bicep"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvDetails(gomock.Any(), radcli.TestEnvironmentName).
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - fallback workspace invalid",
			Input:         []string{"app.bicep"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad run - too many args",
			Input:         []string{"app.bicep", "anotherfile.json"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	// NOTE: most of the logic of this command is shared with the `rad deploy` command.
	// We're using one of the same tests here as a smoke test, but the bulk of the testing
	// is part of the `rad deploy` tests.
	//
	// We should revisit the test strategy if the code paths deviate sigificantly.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	bicep := bicep.NewMockInterface(ctrl)
	bicep.EXPECT().
		PrepareTemplate("app.bicep").
		Return(map[string]any{}, nil).
		Times(1)

	deployOptionsChan := make(chan deploy.Options, 1)
	deployMock := deploy.NewMockInterface(ctrl)
	deployMock.EXPECT().
		DeployWithProgress(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, o deploy.Options) (clients.DeploymentResult, error) {
			// Capture options for verification
			deployOptionsChan <- o
			close(deployOptionsChan)

			return clients.DeploymentResult{}, nil
		}).
		Times(1)

	portforwardOptionsChan := make(chan portforward.Options, 1)
	portforwardMock := portforward.NewMockInterface(ctrl)
	portforwardMock.EXPECT().
		Run(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, o portforward.Options) error {
			// Capture options for verification
			portforwardOptionsChan <- o
			close(portforwardOptionsChan)

			// Wait for context to be canceled
			<-ctx.Done()

			// Run is expected to close this channel.
			close(o.StatusChan)
			return ctx.Err()
		}).
		Times(1)

	logstreamOptionsChan := make(chan logstream.Options, 1)
	logstreamMock := logstream.NewMockInterface(ctrl)
	logstreamMock.EXPECT().
		Stream(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, o logstream.Options) error {
			// Capture options for verification
			logstreamOptionsChan <- o
			close(logstreamOptionsChan)

			// Wait for context to be canceled
			<-ctx.Done()
			return ctx.Err()
		}).
		Times(1)

	app := v20231001preview.ApplicationResource{
		Properties: &v20231001preview.ApplicationProperties{
			Status: &v20231001preview.ResourceStatus{
				Compute: &v20231001preview.KubernetesCompute{
					Kind:      to.Ptr("kubernetes"),
					Namespace: to.Ptr("test-namespace-app"),
				},
			},
		},
	}

	clientMock := clients.NewMockApplicationsManagementClient(ctrl)
	clientMock.EXPECT().
		GetEnvDetails(gomock.Any(), "test-environment").
		Return(v20231001preview.EnvironmentResource{}, nil).
		Times(1)
	clientMock.EXPECT().
		CreateApplicationIfNotFound(gomock.Any(), "test-application", gomock.Any()).
		Return(nil).
		Times(1)
	clientMock.EXPECT().
		ShowApplication(gomock.Any(), "test-application").
		Return(app, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name: "kind-kind",
	}
	outputSink := &output.MockOutput{}
	providers := &clients.Providers{
		Radius: &clients.RadiusProvider{
			EnvironmentID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			ApplicationID: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s/applications/test-application", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
		},
	}
	runner := &Runner{
		Runner: deploycmd.Runner{
			Bicep:  bicep,
			Deploy: deployMock,
			Output: outputSink,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: clientMock,
			},

			FilePath:        "app.bicep",
			ApplicationName: "test-application",
			EnvironmentName: radcli.TestEnvironmentName,
			Parameters:      map[string]map[string]any{},
			Workspace:       workspace,
			Providers:       providers,
		},
		Logstream:   logstreamMock,
		Portforward: portforwardMock,
	}

	// We'll run the actual command in the background, and do cancellation and verification in
	// the foreground.
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	resultErrChan := make(chan error, 1)
	go func() {
		resultErrChan <- runner.Run(ctx)
	}()

	deployOptions := <-deployOptionsChan
	// Deployment is scoped to app and env
	require.Equal(t, runner.Providers.Radius.ApplicationID, deployOptions.Providers.Radius.ApplicationID)
	require.Equal(t, runner.Providers.Radius.EnvironmentID, deployOptions.Providers.Radius.EnvironmentID)

	logStreamOptions := <-logstreamOptionsChan
	// Logstream is scoped to application and namespace
	require.Equal(t, runner.ApplicationName, logStreamOptions.ApplicationName)
	require.Equal(t, "kind-kind", logStreamOptions.KubeContext)
	require.Equal(t, "test-namespace-app", logStreamOptions.Namespace)

	portforwardOptions := <-portforwardOptionsChan
	// Port-forward is scoped to application and namespace
	require.Equal(t, runner.ApplicationName, portforwardOptions.ApplicationName)
	require.Equal(t, "kind-kind", portforwardOptions.KubeContext)
	require.Equal(t, "test-namespace-app", portforwardOptions.Namespace)

	// Shut down the log stream and verify result
	cancel()
	err := <-resultErrChan
	require.NoError(t, err)

	// All of the output in this command is being done by functions that we mock for testing, so this
	// is always empty except for some boilerplate.
	expected := []any{
		output.LogOutput{
			Format: "",
		},
		output.LogOutput{
			Format: "Starting log stream...",
		},
		output.LogOutput{
			Format: "",
		},
	}
	require.Equal(t, expected, outputSink.Writes)
}
