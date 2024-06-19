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
			Input:         []string{},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many arguments",
			Input:         []string{"dddd"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceProviders := []v20231001preview.ResourceProviderResource{
			{
				Name: to.Ptr("Applications.Test1"),
				Properties: &v20231001preview.ResourceProviderProperties{
					ResourceTypes: []*v20231001preview.ResourceType{
						{
							ResourceType: to.Ptr("exampleResources1"),
							APIVersions: map[string]*v20231001preview.ResourceTypeAPIVersion{
								"2023-10-01-preview": {},
							},
						},
					},
				},
			},
			{
				Name: to.Ptr("Applications.Test2"),
				Properties: &v20231001preview.ResourceProviderProperties{
					ResourceTypes: []*v20231001preview.ResourceType{
						{
							ResourceType: to.Ptr("exampleResources2"),
							APIVersions: map[string]*v20231001preview.ResourceTypeAPIVersion{
								"2023-10-01-preview": {},
							},
						},
						{
							ResourceType: to.Ptr("exampleResources3"),
							APIVersions: map[string]*v20231001preview.ResourceTypeAPIVersion{
								"2023-10-01-preview": {},
							},
						},
					},
				},
			},
		}

		resourceTypes := []common.ResourceType{
			{
				Name:                      "Applications.Test1/exampleResources1",
				ResourceProviderNamespace: "Applications.Test1",
				APIVersions:               []string{"2023-10-01-preview"},
			},
			{
				Name:                      "Applications.Test2/exampleResources2",
				ResourceProviderNamespace: "Applications.Test2",
				APIVersions:               []string{"2023-10-01-preview"},
			},
			{
				Name:                      "Applications.Test2/exampleResources3",
				ResourceProviderNamespace: "Applications.Test2",
				APIVersions:               []string{"2023-10-01-preview"},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			ListResourceProviders(gomock.Any(), "local").
			Return(resourceProviders, nil).
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
			Format:            "table",
			Output:            outputSink,
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     resourceTypes,
				Options: common.GetResourceTypeTableFormat(),
			},
		}

		require.Equal(t, expected, outputSink.Writes)
	})
}
