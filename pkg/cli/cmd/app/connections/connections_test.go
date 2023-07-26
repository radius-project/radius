// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package connections

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/golang/mock/gomock"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/clients_new/generated"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/to"
	"github.com/project-radius/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	application := v20220315privatepreview.ApplicationResource{
		Name: to.Ptr("test-app"),
		ID:   to.Ptr(applicationResourceID),
		Type: to.Ptr("Applications.Core/applications"),
		Properties: &v20220315privatepreview.ApplicationProperties{
			Environment: to.Ptr(environmentResourceID),
		},
	}

	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Connections command application (positional)",
			Input:         []string{"test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					ShowApplication(gomock.Any(), "test-app").
					Return(application, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				// These values are used by Run()
				require.Equal(t, "test-app", runner.ApplicationName)
				require.Equal(t, "test-env", runner.EnvironmentName)
			},
		},
		{
			Name:          "Connections command application (flag)",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					ShowApplication(gomock.Any(), "test-app").
					Return(application, nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				// These values are used by Run()
				require.Equal(t, "test-app", runner.ApplicationName)
				require.Equal(t, "test-env", runner.EnvironmentName)
			},
		},
		{
			Name:          "Connections command missing application",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.ApplicationManagementClient.EXPECT().
					ShowApplication(gomock.Any(), "test-app").
					Return(v20220315privatepreview.ApplicationResource{}, &azcore.ResponseError{ErrorCode: v1.CodeNotFound}).
					Times(1)
			},
		},
		{
			Name:          "Connections command with incorrect args",
			Input:         []string{"foo", "bar"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	// This example is a very simple example of the application graph as an integration test.
	// The unit tests for this package cover the more complex cases.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	applicationResources := []generated.GenericResource{
		{
			ID:         to.Ptr(containerResourceID),
			Properties: makeResourceProperties(map[string]string{"db": redisResourceID}, []any{containerDeploymentOutputResource}),
		},
	}
	environmentResources := []generated.GenericResource{
		{
			ID:         to.Ptr(redisResourceID),
			Properties: makeResourceProperties(nil, []any{redisAWSOutputResource}),
		},
	}

	appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
	appManagementClient.EXPECT().
		ListAllResourcesByApplication(gomock.Any(), "test-app").
		Return(applicationResources, nil).
		Times(1)
	appManagementClient.EXPECT().
		ListAllResourcesByEnvironment(gomock.Any(), "test-env").
		Return(environmentResources, nil).
		Times(1)

	workspace := &workspaces.Workspace{
		Connection: map[string]any{
			"kind":    "kubernetes",
			"context": "kind-kind",
		},
		Name:  "kind-kind",
		Scope: "/planes/radius/local/resourceGroups/test-group",
	}
	outputSink := &output.MockOutput{}
	runner := &Runner{
		ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
		Workspace:         workspace,
		Output:            outputSink,

		// Populated by Validate()
		ApplicationName: "test-app",
		EnvironmentName: "test-env",
	}

	err := runner.Run(context.Background())
	require.NoError(t, err)

	expectedOutput := `Displaying application: test-app

Name: webapp (Applications.Core/containers)
Connections:
  webapp -> redis (Applications.Datastores/redisCaches)
Resources:
  demo (kubernetes: apps/Deployment)

Name: redis (Applications.Datastores/redisCaches)
Connections:
  webapp (Applications.Core/containers) -> redis
Resources:
  redis-aqbjixghynqgg (aws: AWS.MemoryDB/Cluster)

`

	expected := []any{
		output.LogOutput{
			Format: expectedOutput,
		},
	}

	require.Equal(t, expected, outputSink.Writes)
}
