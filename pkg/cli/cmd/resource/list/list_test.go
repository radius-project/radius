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
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/to"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
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
			Name:          "Valid List Command with environment",
			Input:         []string{"Applications.Core/containers", "-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with full environment flag",
			Input:         []string{"Applications.Core/containers", "--environment", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with environment only (no resource type)",
			Input:         []string{"-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with environment only (no resource type) using full environment flag",
			Input:         []string{"--environment", "test-env"},
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
			Name:          "List Command with no arguments",
			Input:         []string{},
			ExpectedValid: true,
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
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("List resources by type in application", func(t *testing.T) {
		t.Run("Application does not exist", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			appManagementClient.EXPECT().
				GetResourceProviderSummary(context.Background(), "local", "Applications.Core").
				Return(ucp.ResourceProviderSummary{
					Name: to.Ptr("Applications.Core"),
					ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
						"containers": {
							APIVersions: map[string]map[string]any{
								"2023-01-01": {},
							},
							DefaultAPIVersion: to.Ptr("2023-01-01"),
						},
					},
					Locations: map[string]map[string]any{
						"east": {},
					},
				}, nil).Times(1)

			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "test-app").
				Return(v20231001preview.ApplicationResource{}, radcli.Create404Error()).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName},
				ApplicationName:           "test-app",
				ResourceType:              "Applications.Core/containers",
				Format:                    "table",
				ResourceTypeSuffix:        "containers",
				ResourceProviderNameSpace: "Applications.Core",
			}

			err := runner.Run(context.Background())
			require.Error(t, err)
			require.IsType(t, err, clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", "test-app", radcli.TestWorkspaceName))
		})

		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("containers", "A"),
				radcli.CreateResource("containers", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResourceProviderSummary(context.Background(), "local", "Applications.Core").
				Return(ucp.ResourceProviderSummary{
					Name: to.Ptr("Applications.Core"),
					ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
						"containers": {
							APIVersions: map[string]map[string]any{
								"2023-01-01": {},
							},
							DefaultAPIVersion: to.Ptr("2023-01-01"),
						},
					},
					Locations: map[string]map[string]any{
						"east": {},
					},
				}, nil).Times(1)
			appManagementClient.EXPECT().
				GetApplication(gomock.Any(), "test-app").
				Return(v20231001preview.ApplicationResource{}, nil).Times(1)
			appManagementClient.EXPECT().
				ListResourcesOfTypeInApplication(gomock.Any(), "test-app", "Applications.Core/containers").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{},
				ApplicationName:           "test-app",
				ResourceType:              "Applications.Core/containers",
				Format:                    "table",
				ResourceTypeSuffix:        "containers",
				ResourceProviderNameSpace: "Applications.Core",
			}

			err := runner.Run(context.Background())
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
				radcli.CreateResource("Applications.Core/containers", "A"),
				radcli.CreateResource("Applications.Core/containers", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResourceProviderSummary(context.Background(), "local", "Applications.Core").
				Return(ucp.ResourceProviderSummary{
					Name: to.Ptr("Applications.Core"),
					ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
						"containers": {
							APIVersions: map[string]map[string]any{
								"2023-01-01": {},
							},
							DefaultAPIVersion: to.Ptr("2023-01-01"),
						},
					},
					Locations: map[string]map[string]any{
						"east": {},
					},
				}, nil).Times(1)

			appManagementClient.EXPECT().
				ListResourcesOfType(gomock.Any(), "Applications.Core/containers").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{},
				ApplicationName:           "",
				ResourceType:              "Applications.Core/containers",
				Format:                    "table",
				ResourceTypeSuffix:        "containers",
				ResourceProviderNameSpace: "Applications.Core",
			}

			err := runner.Run(context.Background())
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
				radcli.CreateResource("Applications.Core/containers", "A"),
				radcli.CreateResource("Applications.Core/containers", "B"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResourceProviderSummary(context.Background(), "local", "Applications.Core").
				Return(ucp.ResourceProviderSummary{
					Name: to.Ptr("Applications.Core"),
					ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
						"containers": {
							APIVersions: map[string]map[string]any{
								"2023-01-01": {},
							},
							DefaultAPIVersion: to.Ptr("2023-01-01"),
						},
					},
					Locations: map[string]map[string]any{
						"east": {},
					},
				}, nil).Times(1)

			appManagementClient.EXPECT().
				ListResourcesOfTypeInEnvironment(gomock.Any(), "test-env", "Applications.Core/containers").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{},
				ApplicationName:           "",
				EnvironmentName:           "test-env",
				ResourceType:              "Applications.Core/containers",
				Format:                    "table",
				ResourceTypeSuffix:        "containers",
				ResourceProviderNameSpace: "Applications.Core",
			}

			err := runner.Run(context.Background())
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

	t.Run("List all resources in environment", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			resources := []generated.GenericResource{
				radcli.CreateResource("Applications.Core/containers", "A"),
				radcli.CreateResource("Applications.Core/containers", "B"),
				radcli.CreateResource("Applications.Core/gateways", "C"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

			appManagementClient.EXPECT().
				ListResourcesInEnvironment(gomock.Any(), "test-env").
				Return(resources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:            outputSink,
				Workspace:         &workspaces.Workspace{},
				ApplicationName:   "",
				EnvironmentName:   "test-env",
				ResourceType:      "",
				Format:            "table",
			}

			err := runner.Run(context.Background())
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

	t.Run("No resource type, application or environment specified", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			ApplicationName:   "",
			EnvironmentName:   "",
			ResourceType:      "",
			Format:            "table",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("Please specify a resource type, application name, or environment name"), err)
	})
}
