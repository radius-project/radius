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

package show

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid",
			Input:         []string{"Applications.Test/exampleResources"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: not a resource type",
			Input:         []string{"Applications.Test"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many arguments",
			Input:         []string{"Applications.Test/exampleResources", "dddd"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: not enough many arguments",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	resourceProvider := v20231001preview.ResourceProviderSummary{
		Name: to.Ptr("Applications.Test"),
		ResourceTypes: map[string]*v20231001preview.ResourceProviderSummaryResourceType{
			"exampleResources": {
				APIVersions: map[string]*v20231001preview.ResourceTypeSummaryResultAPIVersion{
					"2023-10-01-preview": {},
				},
			},
		},
	}

	t.Run("Success: Resource Type Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceType := common.ResourceType{
			Name:                      "Applications.Test/exampleResources",
			ResourceProviderNamespace: "Applications.Test",
			APIVersions:               map[string]*common.APIVersionProperties{"2023-10-01-preview": {}},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Test").
			Return(resourceProvider, nil).
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
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:                 workspace,
			Format:                    "table",
			Output:                    outputSink,
			ResourceTypeName:          "Applications.Test/exampleResources",
			ResourceProviderNamespace: "Applications.Test",
			ResourceTypeSuffix:        "exampleResources",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     resourceType,
				Options: common.GetResourceTypeTableFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Error: Resource Provider Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.AnotherTest").
			Return(v20231001preview.ResourceProviderSummary{}, radcli.Create404Error()).
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
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:                 workspace,
			Format:                    "table",
			Output:                    outputSink,
			ResourceTypeName:          "Applications.AnotherTest/exampleResources",
			ResourceProviderNamespace: "Applications.AnotherTest",
			ResourceTypeSuffix:        "exampleResources",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("The resource provider \"Applications.AnotherTest\" was not found or has been deleted."), err)

		require.Empty(t, outputSink.Writes)
	})

	t.Run("Error: Resource Type Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "Applications.Test").
			Return(resourceProvider, nil).
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
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Workspace:                 workspace,
			Format:                    "table",
			Output:                    outputSink,
			ResourceTypeName:          "Applications.Test/anotherResources",
			ResourceProviderNamespace: "Applications.Test",
			ResourceTypeSuffix:        "anotherResources",
		}

		err := runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("Resource type \"anotherResources\" not found in resource provider \"Applications.Test\"."), err)

		require.Empty(t, outputSink.Writes)
	})
}
