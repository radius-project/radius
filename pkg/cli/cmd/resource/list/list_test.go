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
			Name:          "Valid List Command with resource type",
			Input:         []string{"Applications.Core/containers"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command without resource type",
			Input:         []string{},
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
			Name:          "Valid List Command with group",
			Input:         []string{"Applications.Core/containers", "-g", "my-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with all filters",
			Input:         []string{"Applications.Core/containers", "-g", "my-group", "-e", "test-env", "-a", "test-app"},
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
			Input:         []string{"Applications.Core/containers", "foo"},
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
	// Helper function to create test resources
	createTestResources := func(resourceType string, names ...string) []generated.GenericResource {
		var resources []generated.GenericResource
		for _, name := range names {
			resources = append(resources, radcli.CreateResource(resourceType, name))
		}
		return resources
	}

	// Helper function to create test resources with application
	createTestResourcesWithApp := func(resourceType string, appID string, names ...string) []generated.GenericResource {
		var resources []generated.GenericResource
		for _, name := range names {
			resource := radcli.CreateResource(resourceType, name)
			if resource.Properties == nil {
				resource.Properties = make(map[string]interface{})
			}
			resource.Properties["application"] = appID
			resources = append(resources, resource)
		}
		return resources
	}

	// Helper function to create test resources with environment
	createTestResourcesWithEnv := func(resourceType string, envID string, names ...string) []generated.GenericResource {
		var resources []generated.GenericResource
		for _, name := range names {
			resource := radcli.CreateResource(resourceType, name)
			if resource.Properties == nil {
				resource.Properties = make(map[string]interface{})
			}
			resource.Properties["environment"] = envID
			resources = append(resources, resource)
		}
		return resources
	}

	testCases := []struct {
		name              string
		runner            *Runner
		setupMocks        func(*gomock.Controller) clients.ApplicationsManagementClient
		expectedResources []generated.GenericResource
		expectedError     error
	}{
		{
			name: "List by resource type only",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				resources := createTestResources("testResources", "A", "B", "C")
				client.EXPECT().
					ListResourcesOfType(gomock.Any(), "MyCompany.Resources/testResources").
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResources("testResources", "A", "B", "C"),
		},
		{
			name: "List by application only with resource type",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				ApplicationName:           "test-app",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{}, nil)
				resources := createTestResourcesWithApp("testResources", appID, "A", "B")
				client.EXPECT().
					ListResourcesOfTypeInApplication(gomock.Any(), appID, "MyCompany.Resources/testResources").
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResourcesWithApp("testResources", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app", "A", "B"),
		},
		{
			name: "List by environment only with resource type",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				EnvironmentName:           "test-env",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				resources := createTestResourcesWithEnv("testResources", envID, "A", "B", "C")
				client.EXPECT().
					ListResourcesOfTypeInEnvironment(gomock.Any(), envID, "MyCompany.Resources/testResources").
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResourcesWithEnv("testResources", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env", "A", "B", "C"),
		},
		{
			name: "List by group only with resource type",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				GroupName:                 "another-group",
				PlaneName:                 "local",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				resources := createTestResources("testResources", "A", "B")
				client.EXPECT().
					ListResourcesOfTypeInResourceGroup(gomock.Any(), "local", "another-group", "MyCompany.Resources/testResources").
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResources("testResources", "A", "B"),
		},
		{
			name: "List with group and environment filters",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				GroupName:                 "another-group",
				EnvironmentName:           "test-env",
				PlaneName:                 "local",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				resources := createTestResourcesWithEnv("testResources", envID, "A", "B")
				client.EXPECT().
					ListResourcesOfTypeInResourceGroupFiltered(gomock.Any(), "local", "another-group", "MyCompany.Resources/testResources", envID, "").
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResourcesWithEnv("testResources", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env", "A", "B"),
		},
		{
			name: "List with all three filters",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				GroupName:                 "another-group",
				EnvironmentName:           "test-env",
				ApplicationName:           "test-app",
				PlaneName:                 "local",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"

				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{}, nil)

				resources := createTestResourcesWithApp("testResources", appID, "A")
				client.EXPECT().
					ListResourcesOfTypeInResourceGroupFiltered(gomock.Any(), "local", "another-group", "MyCompany.Resources/testResources", envID, appID).
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResourcesWithApp("testResources", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app", "A"),
		},
		{
			name: "List all resources (no type, no filters)",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				client.EXPECT().
					ListAllResourceTypesNames(gomock.Any(), "local").
					Return([]string{"MyCompany.Resources/type1", "MyCompany.Resources/type2"}, nil)

				resources1 := createTestResources("type1", "A")
				resources2 := createTestResources("type2", "B", "C")

				client.EXPECT().
					ListResourcesOfType(gomock.Any(), "MyCompany.Resources/type1").
					Return(resources1, nil)
				client.EXPECT().
					ListResourcesOfType(gomock.Any(), "MyCompany.Resources/type2").
					Return(resources2, nil)

				return client
			},
			expectedResources: append(createTestResources("type1", "A"), createTestResources("type2", "B", "C")...),
		},
		{
			name: "List with environment and application but no resource type",
			runner: &Runner{
				Workspace:       &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				EnvironmentName: "test-env",
				ApplicationName: "test-app",
				PlaneName:       "local",
				Format:          "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"

				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{}, nil)

				// Should list all resources in environment then filter by app
				allResources := []generated.GenericResource{}
				// Add resources with app
				resourcesWithApp := createTestResourcesWithApp("type1", appID, "A", "B")
				allResources = append(allResources, resourcesWithApp...)
				// Add resources without app
				resourcesWithoutApp := createTestResources("type1", "C", "D")
				allResources = append(allResources, resourcesWithoutApp...)

				client.EXPECT().
					ListResourcesInEnvironment(gomock.Any(), envID).
					Return(allResources, nil)

				return client
			},
			expectedResources: createTestResourcesWithApp("type1", "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app", "A", "B"),
		},
		{
			name: "Application not found error",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				ApplicationName:           "non-existent-app",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/non-existent-app"
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{}, radcli.Create404Error())
				return client
			},
			expectedError: clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", "non-existent-app", radcli.TestWorkspaceName),
		},
		{
			name: "Environment not found error",
			runner: &Runner{
				Workspace:                 &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				EnvironmentName:           "non-existent-env",
				ResourceType:              "MyCompany.Resources/testResources",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/non-existent-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, radcli.Create404Error())
				return client
			},
			expectedError: clierrors.Message("The environment %q could not be found in workspace %q. Make sure you specify the correct environment with '-e/--environment'.", "non-existent-env", radcli.TestWorkspaceName),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			appManagementClient := tc.setupMocks(ctrl)
			outputSink := &output.MockOutput{}

			clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
			require.NoError(t, err)

			tc.runner.ConnectionFactory = &connections.MockFactory{ApplicationsManagementClient: appManagementClient}
			tc.runner.UCPClientFactory = clientFactory
			tc.runner.Output = outputSink

			err = tc.runner.Run(context.Background())

			if tc.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err)
				expected := []any{
					output.FormattedOutput{
						Format:  "table",
						Obj:     tc.expectedResources,
						Options: objectformats.GetGenericResourceTableFormat(),
					},
				}
				require.Equal(t, expected, outputSink.Writes)
			}
		})
	}
}
