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

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

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
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/testcontext"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes/fake"
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
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod", "-a", "my-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},

		{
			Name:          "rad run - app set by directory config",
			Input:         []string{"app.bicep", "-e", "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod"},
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
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod").
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
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/test-environment").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - missing env succeeds when template creates it",
			Input:         []string{"app.bicep", "-a", "my-app", "--group", "new-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				templateWithEnv := map[string]any{
					"resources": map[string]any{
						"env": map[string]any{
							"type": "Applications.Core/environments@2023-10-01-preview",
						},
					},
				}
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(templateWithEnv, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - template creates environment but app required",
			Input:         []string{"env.bicep", "--group", "dev"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("env.bicep").
					Return(map[string]any{
						"resources": map[string]any{
							"env": map[string]any{
								"type": "Applications.Core/environments@2023-10-01-preview",
								"name": "dev",
							},
						},
					}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - no env in config, no env flag, no env in template invalid",
			Input:         []string{"app.bicep", "-a", "my-app", "--group", "test-group"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - no env in config, env flag provided, no env in template valid",
			Input:         []string{"app.bicep", "-a", "my-app", "-e", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/prod", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(map[string]any{}, nil).
					Times(1)
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/prod").
					Return(v20231001preview.EnvironmentResource{}, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - no env in config, no env flag, env in template valid",
			Input:         []string{"app.bicep", "-a", "my-app", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				templateWithEnv := map[string]any{
					"resources": map[string]any{
						"env": map[string]any{
							"type": "Applications.Core/environments@2023-10-01-preview",
						},
					},
				}
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(templateWithEnv, nil).
					Times(1)
			},
		},
		{
			Name:          "rad run - no env in config, env flag provided, env in template valid",
			Input:         []string{"app.bicep", "-a", "my-app", "-e", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/prod", "--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				templateWithEnv := map[string]any{
					"resources": map[string]any{
						"env": map[string]any{
							"type": "Applications.Core/environments@2023-10-01-preview",
						},
					},
				}
				mocks.Bicep.EXPECT().
					PrepareTemplate("app.bicep").
					Return(templateWithEnv, nil).
					Times(1)
				// When env flag is explicitly provided, we honor it and validate even if template creates environment
				mocks.ApplicationManagementClient.EXPECT().
					GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/prod").
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

func Test_ValidateWithFakeEnvServer(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)

	// Test case: rad run with valid environment using fake env server
	t.Run("rad run - valid with app and env using fake env server", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create a fake env server that returns valid environment
		validEnvServer := corerpfake.EnvironmentsServer{
			Get: test_client_factory.WithEnvironmentServerNoError().Get,
		}

		workspace := &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
		}

		// Create test client factory with fake env server
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			func() corerpfake.EnvironmentsServer {
				return validEnvServer
			},
			nil,
		)
		require.NoError(t, err)

		// Set up Applications.Core mock to return 404
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/prod").
			Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error()).Times(1)

		// Set up Bicep mock to return empty template
		mockBicep := bicep.NewMockInterface(ctrl)
		mockBicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		f := &framework.Impl{
			ConfigHolder: &framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			Output: &output.MockOutput{},
		}

		cmd, runner := NewCommand(f)
		r := runner.(*Runner)
		r.Workspace = workspace
		r.RadiusCoreClientFactory = factory
		r.ConnectionFactory = &connections.MockFactory{ApplicationsManagementClient: mockAppClient}
		r.Bicep = mockBicep

		// Parse the flags manually to set the environment and app flags
		cmd.SetArgs([]string{"app.bicep", "-e", "prod", "-a", "my-app"})
		cmd.SetContext(context.Background())
		err = cmd.ParseFlags([]string{"-e", "prod", "-a", "my-app"})
		require.NoError(t, err)

		// This should successfully validate
		err = r.Validate(cmd, []string{"app.bicep"})
		require.NoError(t, err, "Run should succeed with valid environment and app")
	})

	// Test case: rad run with environment that returns 404 from both providers should fail
	t.Run("rad run - env specified with -e returns 404 from both providers should fail", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Create a fake env server that returns 404
		nonExistentEnvServer := corerpfake.EnvironmentsServer{
			Get: func(
				_ context.Context,
				_ string,
				_ *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				errResp.SetError(fmt.Errorf("Environment not found"))
				errResp.SetResponseError(404, "Not Found")
				return
			},
		}

		workspace := &workspaces.Workspace{
			Name:  "test-workspace",
			Scope: "/planes/radius/local/resourceGroups/test-resource-group",
		}

		// Create test client factory with fake env server
		factory, err := test_client_factory.NewRadiusCoreTestClientFactory(
			workspace.Scope,
			func() corerpfake.EnvironmentsServer {
				return nonExistentEnvServer
			},
			nil,
		)
		require.NoError(t, err)

		// Set up Applications.Core mock to also return 404
		mockAppClient := clients.NewMockApplicationsManagementClient(ctrl)
		mockAppClient.EXPECT().
			GetEnvironment(gomock.Any(), "/planes/radius/local/resourceGroups/test-resource-group/providers/Applications.Core/environments/nonexistent").
			Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error()).
			Times(1)

		// Set up Bicep mock to return empty template
		mockBicep := bicep.NewMockInterface(ctrl)
		mockBicep.EXPECT().
			PrepareTemplate("app.bicep").
			Return(map[string]any{}, nil).
			Times(1)

		f := &framework.Impl{
			ConfigHolder: &framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			Output: &output.MockOutput{},
		}

		cmd, runner := NewCommand(f)
		r := runner.(*Runner)
		r.Workspace = workspace
		r.RadiusCoreClientFactory = factory
		r.ConnectionFactory = &connections.MockFactory{ApplicationsManagementClient: mockAppClient}
		r.Bicep = mockBicep

		// Parse the flags manually to set the environment flag with a non-existent environment
		cmd.SetArgs([]string{"app.bicep", "-e", "nonexistent", "-a", "my-app"})
		cmd.SetContext(context.Background())
		err = cmd.ParseFlags([]string{"-e", "nonexistent", "-a", "my-app"})
		require.NoError(t, err)

		// This should fail because both providers return 404 and user specified environment name
		err = r.Validate(cmd, []string{"app.bicep"})
		require.Error(t, err, "Run should fail when both Radius.Core and Applications.Core return 404 for specified environment")
		require.Contains(t, err.Error(), "The environment \"nonexistent\" does not exist in scope", "Error should indicate environment doesn't exist")
		require.Contains(t, err.Error(), "Run `rad env create` first", "Error should suggest creating environment")
	})
}

func Test_Run(t *testing.T) {
	// NOTE: most of the logic of this command is shared with the `rad deploy` command.
	// We're using one of the same tests here as a smoke test, but the bulk of the testing
	// is part of the `rad deploy` tests.
	//
	// We should revisit the test strategy if the code paths deviate sigificantly.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	portforwardMock := portforward.NewMockInterface(ctrl)

	dashboardDeployment := createDashboardDeploymentObject()
	fakeKubernetesClient := fake.NewSimpleClientset(dashboardDeployment)

	appPortforwardOptionsChan := make(chan portforward.Options, 1)
	appLabelSelector, err := portforward.CreateLabelSelectorForApplication("test-application")
	require.NoError(t, err)
	portforwardMock.EXPECT().
		Run(gomock.Any(), PortForwardOptionsMatcher{LabelSelector: appLabelSelector}).
		DoAndReturn(func(ctx context.Context, o portforward.Options) error {
			// Capture options for verification
			appPortforwardOptionsChan <- o
			close(appPortforwardOptionsChan)

			// Run is expected to close this channel
			close(o.StatusChan)

			// Wait for context to be canceled
			<-ctx.Done()
			return ctx.Err()
		}).
		Times(1)

	dashboardPortforwardOptionsChan := make(chan portforward.Options, 1)
	dashboardLabelSelector, err := portforward.CreateLabelSelectorForDashboard()
	require.NoError(t, err)
	portforwardMock.EXPECT().
		Run(gomock.Any(), PortForwardOptionsMatcher{LabelSelector: dashboardLabelSelector}).
		DoAndReturn(func(ctx context.Context, o portforward.Options) error {
			// Capture options for verification
			dashboardPortforwardOptionsChan <- o
			close(dashboardPortforwardOptionsChan)

			// Run is expected to close this channel
			close(o.StatusChan)

			// Wait for context to be canceled
			<-ctx.Done()
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
		CreateApplicationIfNotFound(gomock.Any(), "test-application", gomock.Any()).
		Return(nil).
		Times(1)
	clientMock.EXPECT().
		GetApplication(gomock.Any(), "test-application").
		Return(app, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:        "kind-kind",
		Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
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
			Deploy: deployMock,
			Output: outputSink,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: clientMock,
			},

			FilePath:            "app.bicep",
			ApplicationName:     "test-application",
			EnvironmentNameOrID: radcli.TestEnvironmentName,
			Parameters:          map[string]map[string]any{},
			Template:            map[string]any{}, // Template is prepared in Validate
			Workspace:           workspace,
			Providers:           providers,
		},
		Logstream:        logstreamMock,
		Portforward:      portforwardMock,
		kubernetesClient: fakeKubernetesClient,
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
	require.Equal(t, "test-namespace-app", logStreamOptions.Namespace)

	appPortforwardOptions := <-appPortforwardOptionsChan
	// Application Portforward is scoped to application and app namespace
	require.Equal(t, "kind-kind", appPortforwardOptions.KubeContext)
	require.Equal(t, "test-namespace-app", appPortforwardOptions.Namespace)
	require.Equal(t, "radapp.io/application=test-application", appPortforwardOptions.LabelSelector.String())

	dashboardPortforwardOptions := <-dashboardPortforwardOptionsChan
	// Dashboard Portforward is scoped to dashboard and radius namespace
	require.Equal(t, "kind-kind", dashboardPortforwardOptions.KubeContext)
	require.Equal(t, "radius-system", dashboardPortforwardOptions.Namespace)
	require.Equal(t, "app.kubernetes.io/name=dashboard,app.kubernetes.io/part-of=radius", dashboardPortforwardOptions.LabelSelector.String())

	// Shut down the log stream and verify result
	cancel()
	err = <-resultErrChan
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

func Test_Run_NoDashboard(t *testing.T) {
	// This is the same test as above, but without expecting the dashboard portforward to be started.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

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

	portforwardMock := portforward.NewMockInterface(ctrl)

	fakeKubernetesClient := fake.NewSimpleClientset()

	appPortforwardOptionsChan := make(chan portforward.Options, 1)
	appLabelSelector, err := portforward.CreateLabelSelectorForApplication("test-application")
	require.NoError(t, err)
	portforwardMock.EXPECT().
		Run(gomock.Any(), PortForwardOptionsMatcher{LabelSelector: appLabelSelector}).
		DoAndReturn(func(ctx context.Context, o portforward.Options) error {
			// Capture options for verification
			appPortforwardOptionsChan <- o
			close(appPortforwardOptionsChan)

			// Run is expected to close this channel
			close(o.StatusChan)

			// Wait for context to be canceled
			<-ctx.Done()
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
		CreateApplicationIfNotFound(gomock.Any(), "test-application", gomock.Any()).
		Return(nil).
		Times(1)
	clientMock.EXPECT().
		GetApplication(gomock.Any(), "test-application").
		Return(app, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:        "kind-kind",
		Environment: fmt.Sprintf("/planes/radius/local/resourceGroups/%s/providers/applications.core/environments/%s", radcli.TestEnvironmentName, radcli.TestEnvironmentName),
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
			Deploy: deployMock,
			Output: outputSink,
			ConnectionFactory: &connections.MockFactory{
				ApplicationsManagementClient: clientMock,
			},

			FilePath:            "app.bicep",
			ApplicationName:     "test-application",
			EnvironmentNameOrID: radcli.TestEnvironmentName,
			Parameters:          map[string]map[string]any{},
			Template:            map[string]any{}, // Template is prepared in Validate
			Workspace:           workspace,
			Providers:           providers,
		},
		Logstream:        logstreamMock,
		Portforward:      portforwardMock,
		kubernetesClient: fakeKubernetesClient,
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
	require.Equal(t, "test-namespace-app", logStreamOptions.Namespace)

	appPortforwardOptions := <-appPortforwardOptionsChan
	// Application Portforward is scoped to application and app namespace
	require.Equal(t, "kind-kind", appPortforwardOptions.KubeContext)
	require.Equal(t, "test-namespace-app", appPortforwardOptions.Namespace)
	require.Equal(t, "radapp.io/application=test-application", appPortforwardOptions.LabelSelector.String())

	// Shut down the log stream and verify result
	cancel()
	err = <-resultErrChan
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

type PortForwardOptionsMatcher struct {
	LabelSelector labels.Selector
}

func (p PortForwardOptionsMatcher) Matches(x interface{}) bool {
	if s, ok := x.(portforward.Options); ok {
		return p.LabelSelector.String() == s.LabelSelector.String()
	}

	return false
}

func (p PortForwardOptionsMatcher) String() string {
	return fmt.Sprintf("expected label selector %s", p.LabelSelector.String())
}

func createDashboardDeploymentObject() *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dashboard",
			Namespace: "radius-system",
			Labels: map[string]string{
				"app.kubernetes.io/name":    "dashboard",
				"app.kubernetes.io/part-of": "radius",
			},
		},
	}
}
