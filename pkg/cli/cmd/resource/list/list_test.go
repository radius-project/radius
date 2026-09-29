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

package list

import (
	"context"
	"net/http"
	"testing"

	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/test_client_factory"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	configWithWorkspaceNoEnvironment := radcli.LoadConfig(t, `
workspaces:
  default: test-workspace
  items:
    test-workspace:
      connection:
        context: test-context
        kind: kubernetes
      scope: /planes/radius/local/resourceGroups/test-resource-group
`)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid List Command",
			Input:         []string{"Applications.Core/containers"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with application",
			Input:         []string{"Applications.Core/containers", "-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with fallback workspace",
			Input:         []string{"Applications.Core/containers", "-g", "my-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "List Command with invalid resource type",
			Input:         []string{"invalidResourceType"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with too many args",
			Input:         []string{"invalidResourceType", "foo"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with no resource type lists the default environment",
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with no resource type and environment flag",
			Input:         []string{"-e", "other-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with no resource type and application flag",
			Input:         []string{"-a", "test-app"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with no resource type and no default environment",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspaceNoEnvironment,
			},
		},
		{
			Name:          "Valid List Command with resource type and environment flag",
			Input:         []string{"Applications.Core/containers", "-e", "other-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with application and environment flags is invalid",
			Input:         []string{"-a", "test-app", "-e", "other-env"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "List Command with resource type, application and environment flags is invalid",
			Input:         []string{"Applications.Core/containers", "-a", "test-app", "-e", "other-env"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_ValidatePreview(t *testing.T) {
	const (
		scope          = "/planes/radius/local/resourceGroups/test-resource-group"
		previewEnvID   = scope + "/providers/Radius.Core/environments/test-environment"
		previewAppID   = scope + "/providers/Radius.Core/applications/test-app"
		legacyEnvID    = scope + "/providers/Applications.Core/environments/test-environment"
		otherPreviewID = scope + "/providers/Radius.Core/environments/other-env"
	)

	config := radcli.LoadConfig(t, `
workspaces:
  default: test-workspace
  items:
    test-workspace:
      connection:
        context: test-context
        kind: kubernetes
      scope: `+scope+`
      environment: `+previewEnvID+`
`)

	newRunner := func(t *testing.T) (*cobra.Command, *Runner) {
		t.Helper()
		factory := &framework.Impl{
			ConfigHolder: &framework.ConfigHolder{Config: config},
			Output:       &output.MockOutput{},
		}
		command, runner := NewPreviewCommand(factory)
		return command, runner.(*Runner)
	}

	t.Run("preserves default Radius.Core environment ID", func(t *testing.T) {
		command, runner := newRunner(t)

		err := runner.Validate(command, []string{})
		require.NoError(t, err)
		require.Equal(t, previewEnvID, runner.EnvironmentNameOrID)
	})

	t.Run("qualifies explicit environment name as Radius.Core", func(t *testing.T) {
		command, runner := newRunner(t)
		require.NoError(t, command.ParseFlags([]string{"-e", "other-env"}))

		err := runner.Validate(command, []string{})
		require.NoError(t, err)
		require.Equal(t, otherPreviewID, runner.EnvironmentNameOrID)
	})

	t.Run("rejects legacy environment ID", func(t *testing.T) {
		command, runner := newRunner(t)
		require.NoError(t, command.ParseFlags([]string{"-e", legacyEnvID}))

		err := runner.Validate(command, []string{})
		require.Error(t, err)
		require.Contains(t, err.Error(), datamodel.EnvironmentResourceType_v20250801preview)
	})

	t.Run("qualifies application name as Radius.Core", func(t *testing.T) {
		command, runner := newRunner(t)
		require.NoError(t, command.ParseFlags([]string{"-a", "test-app"}))

		err := runner.Validate(command, []string{})
		require.NoError(t, err)
		require.Equal(t, previewAppID, runner.ApplicationID)
	})

	t.Run("preserves full Radius.Core application ID", func(t *testing.T) {
		command, runner := newRunner(t)
		require.NoError(t, command.ParseFlags([]string{"-a", previewAppID}))

		err := runner.Validate(command, []string{})
		require.NoError(t, err)
		require.Equal(t, "test-app", runner.ApplicationName)
		require.Equal(t, previewAppID, runner.ApplicationID)
	})

	t.Run("rejects legacy application ID", func(t *testing.T) {
		command, runner := newRunner(t)
		legacyAppID := scope + "/providers/Applications.Core/applications/test-app"
		require.NoError(t, command.ParseFlags([]string{"-a", legacyAppID}))

		err := runner.Validate(command, []string{})
		require.Error(t, err)
		require.Contains(t, err.Error(), datamodel.ApplicationResourceType_v20250801preview)
	})

	t.Run("rejects out-of-scope environment ID", func(t *testing.T) {
		command, runner := newRunner(t)
		otherScopeEnvID := "/planes/radius/local/resourceGroups/other-resource-group/providers/Radius.Core/environments/test-environment"
		require.NoError(t, command.ParseFlags([]string{"-e", otherScopeEnvID}))

		err := runner.Validate(command, []string{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "other-resource-group")
		require.Contains(t, err.Error(), scope)
	})

	t.Run("rejects out-of-scope application ID", func(t *testing.T) {
		command, runner := newRunner(t)
		otherScopeAppID := "/planes/radius/local/resourceGroups/other-resource-group/providers/Radius.Core/applications/test-app"
		require.NoError(t, command.ParseFlags([]string{"-a", otherScopeAppID}))

		err := runner.Validate(command, []string{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "other-resource-group")
		require.Contains(t, err.Error(), scope)
	})
}

func Test_Run(t *testing.T) {
	t.Run("List resources by type in application", func(t *testing.T) {
		t.Run("Application does not exist", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "test-app").
				Return(v20231001preview.ApplicationResource{}, radcli.Create404Error()).Times(1)

			outputSink := &output.MockOutput{}

			clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)
			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				UCPClientFactory:          clientFactory,
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName},
				ApplicationName:           "test-app",
				ResourceType:              "MyCompany.Resources/testResources",
				Format:                    "table",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
			}

			err = runner.Run(t.Context())
			require.Error(t, err)
			require.IsType(t, err, clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", "test-app", radcli.TestWorkspaceName))
		})

		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("testResources", "A"),
				radcli.CreateResource("testResources", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "test-app").
				Return(v20231001preview.ApplicationResource{}, nil).Times(1)
			appManagementClient.EXPECT().
				ListResourcesOfTypeInApplication(gomock.Any(), "test-app", "MyCompany.Resources/testResources").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)
			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				UCPClientFactory:          clientFactory,
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{},
				ApplicationName:           "test-app",
				ResourceType:              "MyCompany.Resources/testResources",
				Format:                    "table",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
			}

			err = runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     resources,
					Options: objectformats.GetGenericResourceTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})

	t.Run("List resources by type without application", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("MyCompany.Resources/testResources", "A"),
				radcli.CreateResource("MyCompany.Resources/testResources", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			appManagementClient.EXPECT().
				ListResourcesOfType(gomock.Any(), "MyCompany.Resources/testResources").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			workspace := &workspaces.Workspace{
				Connection: map[string]any{
					"kind":    "kubernetes",
					"context": "kind-kind",
				},
				Name:  "kind-kind",
				Scope: "/planes/radius/local/resourceGroups/test-group",
			}
			clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)
			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				UCPClientFactory:          clientFactory,
				Output:                    outputSink,
				Workspace:                 workspace,
				ApplicationName:           "",
				ResourceType:              "MyCompany.Resources/testResources",
				Format:                    "table",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
			}

			err = runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     resources,
					Options: objectformats.GetGenericResourceTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})

	t.Run("List resources by type in environment", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("testResources", "A"),
				radcli.CreateResource("testResources", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ListResourcesOfTypeInEnvironment(gomock.Any(), "test-env", "MyCompany.Resources/testResources").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)
			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				UCPClientFactory:          clientFactory,
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName},
				EnvironmentNameOrID:       "test-env",
				ResourceType:              "MyCompany.Resources/testResources",
				Format:                    "table",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
			}

			err = runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     resources,
					Options: objectformats.GetGenericResourceTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})

	t.Run("List all resources without a resource type", func(t *testing.T) {
		t.Run("List resources in an environment - Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("Applications.Core/containers", "A"),
				radcli.CreateResource("Applications.Datastores/redisCaches", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ListResourcesInEnvironment(gomock.Any(), "test-env").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:              outputSink,
				Workspace:           &workspaces.Workspace{Name: radcli.TestWorkspaceName},
				EnvironmentNameOrID: "test-env",
				Format:              "table",
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     resources,
					Options: objectformats.GetGenericResourceTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})

		t.Run("List resources in an application - Application does not exist", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "test-app").
				Return(v20231001preview.ApplicationResource{}, radcli.Create404Error()).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Name: radcli.TestWorkspaceName},
				ApplicationName:   "test-app",
				Format:            "table",
			}

			err := runner.Run(t.Context())
			require.Error(t, err)
			require.IsType(t, clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", "test-app", radcli.TestWorkspaceName), err)
		})

		t.Run("List resources in an application - Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("Applications.Core/containers", "A"),
				radcli.CreateResource("Applications.Datastores/redisCaches", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "test-app").
				Return(v20231001preview.ApplicationResource{}, nil).Times(1)
			appManagementClient.EXPECT().
				ListResourcesInApplication(gomock.Any(), "test-app").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{Name: radcli.TestWorkspaceName},
				ApplicationName:   "test-app",
				Format:            "table",
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)

			expected := []any{
				output.FormattedOutput{
					Format:  "table",
					Obj:     resources,
					Options: objectformats.GetGenericResourceTableFormat(),
				},
			}
			require.Equal(t, expected, outputSink.Writes)
		})
	})

	t.Run("Preview resources", func(t *testing.T) {
		const (
			scope = "/planes/radius/local/resourceGroups/test-group"
			envID = scope + "/providers/Radius.Core/environments/test-env"
			appID = scope + "/providers/Radius.Core/applications/test-app"
		)

		t.Run("List all resources in environment passes full Radius.Core ID", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			resources := []generated.GenericResource{radcli.CreateResource("Radius.Compute/containers", "A")}
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ListResourcesInEnvironment(gomock.Any(), envID).
				Return(resources, nil).
				Times(1)

			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:   &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:              outputSink,
				Workspace:           &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: scope},
				EnvironmentNameOrID: envID,
				Format:              "table",
				Preview:             true,
			}

			err := runner.Run(t.Context())
			require.NoError(t, err)
			require.Equal(t, resources, outputSink.Writes[0].(output.FormattedOutput).Obj)
		})

		t.Run("List typed resources in environment passes full Radius.Core ID", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			resources := []generated.GenericResource{radcli.CreateResource("Radius.Compute/containers", "A")}
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ListResourcesOfTypeInEnvironment(gomock.Any(), envID, "MyCompany.Resources/testResources").
				Return(resources, nil).
				Times(1)

			resourceTypeFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				UCPClientFactory:          resourceTypeFactory,
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: scope},
				EnvironmentNameOrID:       envID,
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
				Preview:                   true,
			}

			err = runner.Run(t.Context())
			require.NoError(t, err)
			require.Equal(t, resources, outputSink.Writes[0].(output.FormattedOutput).Obj)
		})

		t.Run("List all resources in application validates and passes full Radius.Core ID", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			resources := []generated.GenericResource{radcli.CreateResource("Radius.Compute/containers", "A")}
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ListResourcesInApplication(gomock.Any(), appID).
				Return(resources, nil).
				Times(1)

			radiusCoreFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
			require.NoError(t, err)
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				RadiusCoreClientFactory: radiusCoreFactory,
				Output:                  outputSink,
				Workspace:               &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: scope},
				ApplicationName:         "test-app",
				ApplicationID:           appID,
				Format:                  "table",
				Preview:                 true,
			}

			err = runner.Run(t.Context())
			require.NoError(t, err)
			require.Equal(t, resources, outputSink.Writes[0].(output.FormattedOutput).Obj)
		})

		t.Run("List typed resources in application passes full Radius.Core ID", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			resources := []generated.GenericResource{radcli.CreateResource("Radius.Compute/containers", "A")}
			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				ListResourcesOfTypeInApplication(gomock.Any(), appID, "MyCompany.Resources/testResources").
				Return(resources, nil).
				Times(1)

			resourceTypeFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)
			radiusCoreFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, nil, test_client_factory.WithApplicationsServerNoError)
			require.NoError(t, err)
			outputSink := &output.MockOutput{}
			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				UCPClientFactory:          resourceTypeFactory,
				RadiusCoreClientFactory:   radiusCoreFactory,
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: scope},
				ApplicationName:           "test-app",
				ApplicationID:             appID,
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
				Preview:                   true,
			}

			err = runner.Run(t.Context())
			require.NoError(t, err)
			require.Equal(t, resources, outputSink.Writes[0].(output.FormattedOutput).Obj)
		})

		t.Run("Missing Radius.Core application returns user-facing error", func(t *testing.T) {
			notFoundServer := func() corerpfake.ApplicationsServer {
				return corerpfake.ApplicationsServer{
					Get: func(
						context.Context,
						string,
						string,
						*corerpv20250801.ApplicationsClientGetOptions,
					) (resp azfake.Responder[corerpv20250801.ApplicationsClientGetResponse], errResp azfake.ErrorResponder) {
						errResp.SetResponseError(http.StatusNotFound, "NotFound")
						return
					},
				}
			}
			radiusCoreFactory, err := test_client_factory.NewRadiusCoreTestClientFactory(scope, nil, nil, notFoundServer)
			require.NoError(t, err)

			runner := &Runner{
				ConnectionFactory:       &connections.MockFactory{ApplicationsManagementClient: clients.NewMockApplicationsManagementClient(gomock.NewController(t))},
				RadiusCoreClientFactory: radiusCoreFactory,
				Output:                  &output.MockOutput{},
				Workspace:               &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: scope},
				ApplicationName:         "test-app",
				ApplicationID:           appID,
				Format:                  "table",
				Preview:                 true,
			}

			err = runner.Run(t.Context())
			require.Error(t, err)
			require.Contains(t, err.Error(), "could not be found")
		})
	})
}
