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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	v20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_extractPlaneName(t *testing.T) {
	tests := []struct {
		name     string
		scope    string
		expected string
	}{
		{
			name:     "Standard scope",
			scope:    "/planes/radius/local/resourceGroups/test-group",
			expected: "local",
		},
		{
			name:     "Different plane name",
			scope:    "/planes/radius/production/resourceGroups/test-group",
			expected: "production",
		},
		{
			name:     "No radius in scope",
			scope:    "/planes/azure/subscriptions/123/resourceGroups/test",
			expected: "local", // defaults to "local"
		},
		{
			name:     "Empty scope",
			scope:    "",
			expected: "local", // defaults to "local"
		},
		{
			name:     "Malformed scope",
			scope:    "invalid-scope",
			expected: "local", // defaults to "local"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPlaneName(tt.scope)
			require.Equal(t, tt.expected, result)
		})
	}
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
			Name:          "Valid List Command without resource type but with group filter",
			Input:         []string{"--group", "test-group"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
		},
		{
			Name:          "Valid List Command with no parameters (defaults to workspace's active group)",
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
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					ResourceType: "MyCompany.Resources/testResources",
				},
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
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					ApplicationName: "test-app",
					ResourceType:    "MyCompany.Resources/testResources",
				},
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
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					EnvironmentName: "test-env",
					ResourceType:    "MyCompany.Resources/testResources",
				},
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
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:    "another-group",
					ResourceType: "MyCompany.Resources/testResources",
				},
				PlaneName:                 "local",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				// Validation expects GetResourceGroup
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "another-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)
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
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "another-group",
					EnvironmentName: "test-env",
					ResourceType:    "MyCompany.Resources/testResources",
				},
				PlaneName:                 "local",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				// Validation expects GetResourceGroup
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "another-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)
				envID := "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				resources := createTestResourcesWithEnv("testResources", envID, "A", "B")
				client.EXPECT().
					ListResourcesOfTypeInResourceGroupFiltered(gomock.Any(), "local", "another-group", "MyCompany.Resources/testResources", envID, "").
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResourcesWithEnv("testResources", "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/environments/test-env", "A", "B"),
		},
		{
			name: "List with all three filters",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "another-group",
					EnvironmentName: "test-env",
					ApplicationName: "test-app",
					ResourceType:    "MyCompany.Resources/testResources",
				},
				PlaneName:                 "local",
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/environments/test-env"
				appID := "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/applications/test-app"

				// Validation expects GetResourceGroup
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "another-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				// Called twice: once in ID resolution, once for env check
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{
						Properties: &v20231001preview.ApplicationProperties{
							Environment: &envID,
						},
					}, nil).Times(2)

				resources := createTestResourcesWithApp("testResources", appID, "A")
				client.EXPECT().
					ListResourcesOfTypeInResourceGroupFiltered(gomock.Any(), "local", "another-group", "MyCompany.Resources/testResources", envID, appID).
					Return(resources, nil)
				return client
			},
			expectedResources: createTestResourcesWithApp("testResources", "/planes/radius/local/resourceGroups/another-group/providers/Applications.Core/applications/test-app", "A"),
		},
		{
			name: "List all resources (no type, no filters - defaults to workspace's active group)",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				PlaneName: "local",
				Format:    "table",
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName: "test-group", // This is now set by default from workspace's active group in Validate()
				},
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)

				// Validation expects GetResourceGroup
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "test-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)

				resources1 := createTestResources("type1", "A")
				resources2 := createTestResources("type2", "B", "C")

				// When GroupName is set (now default), it calls ListResourcesInResourceGroup
				client.EXPECT().
					ListResourcesInResourceGroup(gomock.Any(), "local", "test-group").
					Return(append(resources1, resources2...), nil)

				return client
			},
			expectedResources: append(createTestResources("type1", "A"), createTestResources("type2", "B", "C")...),
		},
		{
			name: "List with environment and application but no resource type",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					EnvironmentName: "test-env",
					ApplicationName: "test-app",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"

				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)
				// Called twice: once in ID resolution, once for env check
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{
						Properties: &v20231001preview.ApplicationProperties{
							Environment: &envID,
						},
					}, nil).Times(2)

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
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					ApplicationName: "non-existent-app",
					ResourceType:    "MyCompany.Resources/testResources",
				},
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
			expectedError: ResourceNotFoundError{
				ResourceType: "application",
				Name:         "non-existent-app",
				Workspace:    radcli.TestWorkspaceName,
			},
		},
		{
			name: "Environment not found error",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					EnvironmentName: "non-existent-env",
					ResourceType:    "MyCompany.Resources/testResources",
				},
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
			expectedError: ResourceNotFoundError{
				ResourceType: "environment",
				Name:         "non-existent-env",
				Workspace:    radcli.TestWorkspaceName,
			},
		},
		{
			name: "Environment ID group mismatch error",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					EnvironmentName: "/planes/radius/local/resourceGroups/wrong-group/providers/Applications.Core/environments/test-env",
					GroupName:       "another-group",
					ResourceType:    "MyCompany.Resources/testResources",
				},
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				PlaneName:                 "local",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				// Validation expects GetResourceGroup
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "another-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)
				// Since environment ID is a fully qualified ID, validation will check it belongs to the group
				return client
			},
			expectedError: clierrors.Message("The provided environment ID targets resource group %q but --group is set to %q.", "wrong-group", "another-group"),
		},
		{
			name: "List resources with partial failure (some resource types fail)",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName: "test-group",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)

				// Validation expects GetResourceGroup
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "test-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)

				// When group filter is used without resource type, it calls ListResourcesInResourceGroup
				allResources := append(
					append(createTestResources("containers", "container1"),
						createTestResources("gateways", "gateway1")...),
					createTestResources("extenders", "extender1")...)

				client.EXPECT().
					ListResourcesInResourceGroup(gomock.Any(), "local", "test-group").
					Return(allResources, nil)

				return client
			},
			expectedResources: append(
				append(createTestResources("containers", "container1"),
					createTestResources("gateways", "gateway1")...),
				createTestResources("extenders", "extender1")...),
		},
		{
			name: "Validation: non-existent group",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "non-existent-group",
					EnvironmentName: "test-env",
					ApplicationName: "test-app",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				// Group check should fail
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "non-existent-group").
					Return(ucpv20231001.ResourceGroupResource{}, &azcore.ResponseError{StatusCode: 404})
				return client
			},
			expectedError: ResourceNotFoundError{
				ResourceType: "resource group",
				Name:         "non-existent-group",
				Workspace:    radcli.TestWorkspaceName,
			},
		},
		{
			name: "Validation: environment not in specified group",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/default-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "test-group",
					EnvironmentName: "test-env",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				// Group exists
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "test-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)

				// Environment exists but in different group (default-group)
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, &azcore.ResponseError{StatusCode: 404})

				return client
			},
			expectedError: ResourceNotFoundError{
				ResourceType: "environment",
				Name:         "test-env",
				Workspace:    radcli.TestWorkspaceName,
			},
		},
		{
			name: "Validation: application not in specified group",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/default-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "test-group",
					ApplicationName: "test-app",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)
				// Group exists
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "test-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)

				// Application doesn't exist in the specified group
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{}, &azcore.ResponseError{StatusCode: 404})

				return client
			},
			expectedError: ResourceNotFoundError{
				ResourceType: "application",
				Name:         "test-app",
				Workspace:    radcli.TestWorkspaceName,
			},
		},
		{
			name: "Validation: application does not belong to specified environment",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/test-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					EnvironmentName: "test-env",
					ApplicationName: "test-app",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)

				// Environment exists
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)

				// Application exists but belongs to different environment
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
				differentEnvID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/different-env"
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{
						Properties: &v20231001preview.ApplicationProperties{
							Environment: &differentEnvID,
						},
					}, nil).Times(2)

				return client
			},
			expectedError: clierrors.Message("Application %q does not belong to environment %q.", "test-app", "test-env"),
		},
		{
			name: "Validation: all three filters with app not in environment",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/default-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "test-group",
					EnvironmentName: "test-env",
					ApplicationName: "test-app",
				},
				PlaneName: "local",
				Format:    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)

				// Group exists
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "test-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)

				// Environment exists in the group
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)

				// Application exists in the group but belongs to different environment
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
				differentEnvID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/different-env"
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{
						Properties: &v20231001preview.ApplicationProperties{
							Environment: &differentEnvID,
						},
					}, nil).Times(2)

				return client
			},
			expectedError: clierrors.Message("Application %q does not belong to environment %q.", "test-app", "test-env"),
		},
		{
			name: "Validation: successful with all filters and correct relationships",
			runner: &Runner{
				Workspace: &workspaces.Workspace{Name: radcli.TestWorkspaceName, Scope: "/planes/radius/local/resourceGroups/default-group"},
				Filter: struct {
					ApplicationName string
					EnvironmentName string
					GroupName       string
					ResourceType    string
				}{
					GroupName:       "test-group",
					EnvironmentName: "test-env",
					ApplicationName: "test-app",
					ResourceType:    "MyCompany.Resources/testResources",
				},
				ResourceTypeSuffix:        "testResources",
				ResourceProviderNamespace: "MyCompany.Resources",
				PlaneName:                 "local",
				Format:                    "table",
			},
			setupMocks: func(ctrl *gomock.Controller) clients.ApplicationsManagementClient {
				client := clients.NewMockApplicationsManagementClient(ctrl)

				// Group exists
				client.EXPECT().
					GetResourceGroup(gomock.Any(), "local", "test-group").
					Return(ucpv20231001.ResourceGroupResource{}, nil)

				// Environment exists in the group
				envID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/environments/test-env"
				client.EXPECT().
					GetEnvironment(gomock.Any(), envID).
					Return(v20231001preview.EnvironmentResource{}, nil)

				// Application exists in the group and belongs to the environment
				appID := "/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app"
				// Called twice: once in ID resolution, once for env check
				client.EXPECT().
					GetApplication(gomock.Any(), appID).
					Return(v20231001preview.ApplicationResource{
						Properties: &v20231001preview.ApplicationProperties{
							Environment: &envID,
						},
					}, nil).Times(2)

				// After validation succeeds, list resources
				resources := createTestResources("testResources", "A", "B")
				client.EXPECT().
					ListResourcesOfTypeInResourceGroupFiltered(gomock.Any(), "local", "test-group",
						"MyCompany.Resources/testResources", envID, appID).
					Return(resources, nil)

				return client
			},
			expectedResources: createTestResources("testResources", "A", "B"),
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
