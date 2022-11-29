// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package run

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/golang/mock/gomock"
	"github.com/project-radius/radius/pkg/cli/bicep"
	"github.com/project-radius/radius/pkg/cli/clients"
	deploycmd "github.com/project-radius/radius/pkg/cli/cmd/deploy"
	"github.com/project-radius/radius/pkg/cli/config"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/deploy"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes/logstream"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
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
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
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
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
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
					Return(v20220315privatepreview.EnvironmentResource{}, nil).
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
		Return(map[string]interface{}{}, nil).
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

	environment := v20220315privatepreview.EnvironmentResource{
		Properties: &v20220315privatepreview.EnvironmentProperties{
			Compute: &v20220315privatepreview.KubernetesCompute{
				Kind:      to.Ptr(v20220315privatepreview.EnvironmentComputeKindKubernetes),
				Namespace: to.Ptr("test-namespace"),
			},
		},
	}

	clientMock := clients.NewMockApplicationsManagementClient(ctrl)
	clientMock.EXPECT().
		GetEnvDetails(gomock.Any(), radcli.TestEnvironmentName).
		Return(environment, nil).
		Times(1)
	clientMock.EXPECT().
		CreateApplicationIfNotFound(gomock.Any(), "test-application", gomock.Any()).
		Return(nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]interface{}{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name: "kind-kind",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		Runner: deploycmd.Runner{
			Bicep:  bicep,
			Deploy: deployMock,
			Output: outputSink,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: clientMock,
			},

			FilePath:        "app.bicep",
			ApplicationID:   fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/applications/%s", radcli.TestEnvironmentName, "test-application"),
			ApplicationName: "test-application",
			EnvironmentID:   fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
			EnvironmentName: radcli.TestEnvironmentName,
			Parameters:      map[string]map[string]interface{}{},
			Workspace:       workspace,
		},
		Logstream: logstreamMock,
	}

	// We'll run the actual command in the background, and do cancellation and verification in
	// the foreground.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Cancel if any test validation values

	resultErrChan := make(chan error, 1)
	go func() {
		resultErrChan <- runner.Run(ctx)
	}()

	deployOptions := <-deployOptionsChan
	// Deployment is scoped to app and env
	require.Equal(t, runner.ApplicationID, deployOptions.ApplicationID)
	require.Equal(t, runner.EnvironmentID, deployOptions.EnvironmentID)

	logStreamOptions := <-logstreamOptionsChan
	// Logstream is scoped to application and namespace
	require.Equal(t, runner.ApplicationName, logStreamOptions.ApplicationName)
	require.Equal(t, "kind-kind", logStreamOptions.KubeContext)
	require.Equal(t, "test-namespace", logStreamOptions.Namespace)

	// Shut down the log stream and verify result
	cancel()
	err := <-resultErrChan
	require.NoError(t, err)

	// All of the output in this command is being done by functions that we mock for testing, so this
	// is always empty except for some boilerplate.
	expected := []interface{}{
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
