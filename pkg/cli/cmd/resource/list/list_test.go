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
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
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
		}, {
			Name:          "Valid List Command with environment",
			Input:         []string{"Applications.Core/containers", "-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with both application and environment flags",
			Input:         []string{"Applications.Core/containers", "-a", "test-app", "-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with both application and environment using full flags",
			Input:         []string{"Applications.Core/containers", "--application", "test-app", "--environment", "test-env"},
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
			Input:         []string{"Applications.Core/environments", "-e", "test-env"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with environment only (no resource type) using full environment flag",
			Input:         []string{"Applications.Core/environments", "--environment", "test-env"},
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
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
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

			err = runner.Run(context.Background())
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

			err = runner.Run(context.Background())
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

			err = runner.Run(context.Background())
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
							APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
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
				GetEnvironment(gomock.Any(), "test-env").
				Return(v20231001preview.EnvironmentResource{}, nil).Times(1)

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
	t.Run("List resources by type in both application and environment", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			ctrl := gomock.NewController(t)

			// Resources in application
			appResources := []generated.GenericResource{
				radcli.CreateResource("Applications.Core/containers", "A"),
				radcli.CreateResource("Applications.Core/containers", "B"),
				radcli.CreateResource("Applications.Core/containers", "C"),
			}

			// Resources in environment
			envResources := []generated.GenericResource{
				radcli.CreateResource("Applications.Core/containers", "B"),
				radcli.CreateResource("Applications.Core/containers", "C"),
				radcli.CreateResource("Applications.Core/containers", "D"),
			}

			// Expected result: intersection of app and env resources
			expectedResources := []generated.GenericResource{
				radcli.CreateResource("Applications.Core/containers", "B"),
				radcli.CreateResource("Applications.Core/containers", "C"),
			}

			appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
			appManagementClient.EXPECT().
				GetResourceProviderSummary(context.Background(), "local", "Applications.Core").
				Return(ucp.ResourceProviderSummary{
					Name: to.Ptr("Applications.Core"),
					ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
						"containers": {
							APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
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
				GetEnvironment(gomock.Any(), "test-env").
				Return(v20231001preview.EnvironmentResource{}, nil).Times(1)

			appManagementClient.EXPECT().
				ListResourcesOfTypeInApplication(gomock.Any(), "test-app", "Applications.Core/containers").
				Return(appResources, nil).Times(1)

			appManagementClient.EXPECT().
				ListResourcesOfTypeInEnvironment(gomock.Any(), "test-env", "Applications.Core/containers").
				Return(envResources, nil).Times(1)

			outputSink := &output.MockOutput{}

			runner := &Runner{
				ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
				Output:                    outputSink,
				Workspace:                 &workspaces.Workspace{},
				ApplicationName:           "test-app",
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
					Obj:     expectedResources,
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
	t.Run("List resources by type in environment using the getResourcesInEnvironment helper", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		resources := []generated.GenericResource{
			radcli.CreateResource("Applications.Core/containers", "test-container"),
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		// Expect resource type validation
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Core").
			Return(ucp.ResourceProviderSummary{
				Name: to.Ptr("Applications.Core"),
				ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
					"containers": {
						APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
							"2023-01-01": {},
						},
						DefaultAPIVersion: to.Ptr("2023-01-01"),
					},
				},
				Locations: map[string]map[string]any{
					"east": {},
				},
			}, nil).Times(1)

		// First the environment is verified to exist
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "default").
			Return(v20231001preview.EnvironmentResource{}, nil).Times(1)

		// Then we should use our helper method
		appManagementClient.EXPECT().
			ListResourcesOfTypeInEnvironment(gomock.Any(), "default", "Applications.Core/containers").
			Return(resources, nil).Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:                    outputSink,
			Workspace:                 &workspaces.Workspace{},
			ApplicationName:           "",
			EnvironmentName:           "default",
			ResourceType:              "Applications.Core/containers",
			ResourceProviderNameSpace: "Applications.Core",
			ResourceTypeSuffix:        "containers",
			Format:                    "table",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}

// Test_ResourceListEnvironmentComparison is a test that compares the behavior of the resource list command
// with and without the --environment flag to detect differences in filtering behavior
func Test_ResourceListEnvironmentComparison(t *testing.T) {
	t.Run("Compare resource list with and without environment flag", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		// Create test resources - same set used for both calls
		resources := []generated.GenericResource{
			createResourceWithEnvironment("Applications.Core/containers", "container1", "default"),
			createResourceWithEnvironment("Applications.Core/containers", "container2", "default"),
			createResourceWithEnvironment("Applications.Core/containers", "container3", "other-env"),
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		// First test without environment flag
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Core").
			Return(ucp.ResourceProviderSummary{
				Name: to.Ptr("Applications.Core"),
				ResourceTypes: map[string]*ucp.ResourceProviderSummaryResourceType{
					"containers": {
						APIVersions: map[string]*ucp.ResourceTypeSummaryResultAPIVersion{
							"2023-01-01": {},
						},
						DefaultAPIVersion: to.Ptr("2023-01-01"),
					},
				},
				Locations: map[string]map[string]any{
					"east": {},
				},
			}, nil).Times(2) // Called twice - once for each run

		// Without environment flag - expect direct call to ListResourcesOfType
		appManagementClient.EXPECT().
			ListResourcesOfType(gomock.Any(), "Applications.Core/containers").
			Return(resources, nil).Times(1)

		// With environment flag - expect environment verification and then filtering
		appManagementClient.EXPECT().
			GetEnvironment(gomock.Any(), "default").
			Return(v20231001preview.EnvironmentResource{}, nil).Times(1)

		// Expect a call to ListResourcesOfTypeInEnvironment
		appManagementClient.EXPECT().
			ListResourcesOfTypeInEnvironment(gomock.Any(), "default", "Applications.Core/containers").
			Return([]generated.GenericResource{
				resources[0],
				resources[1],
			}, nil).Times(1)

		// First run without environment flag
		outputWithoutEnv := &output.MockOutput{}
		runnerWithoutEnv := &Runner{
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:                    outputWithoutEnv,
			Workspace:                 &workspaces.Workspace{},
			ApplicationName:           "",
			EnvironmentName:           "",
			ResourceType:              "Applications.Core/containers",
			Format:                    "table",
			ResourceTypeSuffix:        "containers",
			ResourceProviderNameSpace: "Applications.Core",
		}

		err := runnerWithoutEnv.Run(context.Background())
		require.NoError(t, err)

		// Second run with environment flag
		outputWithEnv := &output.MockOutput{}
		runnerWithEnv := &Runner{
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:                    outputWithEnv,
			Workspace:                 &workspaces.Workspace{},
			ApplicationName:           "",
			EnvironmentName:           "default",
			ResourceType:              "Applications.Core/containers",
			Format:                    "table",
			ResourceTypeSuffix:        "containers",
			ResourceProviderNameSpace: "Applications.Core",
		}

		err = runnerWithEnv.Run(context.Background())
		require.NoError(t, err)

		// Verify both outputs are equivalent
		withoutEnvOutput, ok := outputWithoutEnv.Writes[0].(output.FormattedOutput)
		require.True(t, ok)

		withEnvOutput, ok := outputWithEnv.Writes[0].(output.FormattedOutput)
		require.True(t, ok)

		// Check the number of resources
		withoutEnvResources, ok := withoutEnvOutput.Obj.([]generated.GenericResource)
		require.True(t, ok)

		withEnvResources, ok := withEnvOutput.Obj.([]generated.GenericResource)
		require.True(t, ok)

		// We expect the environment flag to filter out the resource not in the "default" environment
		require.Len(t, withoutEnvResources, 3, "Without environment flag should show all resources")
		require.Len(t, withEnvResources, 2, "With environment flag should only show resources in the specified environment")
	})
}

// Helper function to create a test resource with an environment property
func createResourceWithEnvironment(resourceType, name, environmentName string) generated.GenericResource {
	resource := radcli.CreateResource(resourceType, name)

	if resource.Properties == nil {
		resource.Properties = make(map[string]any)
	}

	if environmentName != "" {
		// Set the environment property directly as a string (matches the format in isResourceInEnvironment)
		resource.Properties["environment"] = "/planes/radius/local/resourcegroups/default/providers/Applications.Core/environments/" + environmentName
	}

	return resource
}
