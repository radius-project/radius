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

package create

import (
	"context"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
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
			Input:         []string{"--from-file", "testdata/valid.yaml"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: Error in manifest",
			Input:         []string{"--from-file", "testdata/missing-required-field.yaml"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: missing arguments",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many arguments",
			Input:         []string{"abcd", "--from-file", "testdata/valid.yaml"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: resource provider created", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		expectedResourceProvider := v20231001preview.ResourceProviderResource{
			Location:   to.Ptr(v1.LocationGlobal),
			Properties: &v20231001preview.ResourceProviderProperties{},
		}
		expectedResourceType := v20231001preview.ResourceTypeResource{
			Properties: &v20231001preview.ResourceTypeProperties{},
		}
		expectedAPIVersion := v20231001preview.APIVersionResource{
			Properties: &v20231001preview.APIVersionProperties{},
		}
		expectedLocation := v20231001preview.LocationResource{
			Properties: &v20231001preview.LocationProperties{
				ResourceTypes: map[string]*v20231001preview.LocationResourceType{
					"testResources": {
						APIVersions: map[string]map[string]any{
							"2025-01-01-preview": {},
						},
					},
				},
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			CreateOrUpdateResourceProvider(gomock.Any(), "local", "MyCompany.Resources", &expectedResourceProvider).
			Return(expectedResourceProvider, nil).
			Times(1)
		appManagementClient.EXPECT().
			CreateOrUpdateResourceType(gomock.Any(), "local", "MyCompany.Resources", "testResources", &expectedResourceType).
			Return(expectedResourceType, nil).
			Times(1)
		appManagementClient.EXPECT().
			CreateOrUpdateAPIVersion(gomock.Any(), "local", "MyCompany.Resources", "testResources", "2025-01-01-preview", &expectedAPIVersion).
			Return(expectedAPIVersion, nil).
			Times(1)
		appManagementClient.EXPECT().
			CreateOrUpdateLocation(gomock.Any(), "local", "MyCompany.Resources", v1.LocationGlobal, &expectedLocation).
			Return(expectedLocation, nil).
			Times(1)
		appManagementClient.EXPECT().
			GetResourceProvider(gomock.Any(), "local", "MyCompany.Resources").
			Return(expectedResourceProvider, nil).
			Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			ResourceProvider:  resourceProviderData,
			Format:            "table",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		expectedOutput := []any{
			output.LogOutput{
				Format: "Creating resource provider %s",
				Params: []any{"MyCompany.Resources"},
			},
			output.LogOutput{
				Format: "Creating resource type %s/%s",
				Params: []any{"MyCompany.Resources", "testResources"},
			},
			output.LogOutput{
				Format: "Creating API Version %s/%s@%s",
				Params: []any{"MyCompany.Resources", "testResources", "2025-01-01-preview"},
			},
			output.LogOutput{
				Format: "Creating location %s/%s",
				Params: []any{"MyCompany.Resources", "global"},
			},
			output.LogOutput{
				Format: "",
				Params: nil,
			},
			output.FormattedOutput{
				Format:  "table",
				Obj:     expectedResourceProvider,
				Options: common.GetResourceProviderTableFormat(),
			},
		}
		require.Equal(t, expectedOutput, outputSink.Writes)
	})

}
