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
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
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
			Input:         []string{"testResources", "--from-file", "testdata/valid.yaml"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: resource type not present in manifest",
			Input:         []string{"myResources", "--from-file", "testdata/valid.yaml"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: missing resource type as argument",
			Input:         []string{"--from-file", "testdata/valid.yaml"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: resource type created", func(t *testing.T) {
		resourceProvider := v20231001preview.ResourceProviderSummary{
			Name: to.Ptr("MyCompany.Resources"),
			ResourceTypes: map[string]*v20231001preview.ResourceProviderSummaryResourceType{
				"testResources": &v20231001preview.ResourceProviderSummaryResourceType{
					APIVersions: map[string]*v20231001preview.ResourceTypeSummaryResultAPIVersion{
						"2023-10-01-preview": {},
					},
					Capabilities: []*string{},
				},
			},
		}
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			GetResourceProviderSummary(gomock.Any(), "local", "MyCompany.Resources").
			Return(resourceProvider, nil).
			Times(1)

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
		require.NoError(t, err)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			ConnectionFactory:                &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			UCPClientFactory:                 clientFactory,
			Output:                           outputSink,
			Workspace:                        &workspaces.Workspace{},
			ResourceProvider:                 resourceProviderData,
			Format:                           "table",
			ResourceProviderManifestFilePath: "testdata/valid.yaml",
			ResourceTypeName:                 "testResources",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
		expected := []interface{}{
			output.LogOutput{
				Format: "Resource provider %q found. Registering resource type %q.",
				Params: []interface{}{"MyCompany.Resources", "testResources"},
			},
			output.LogOutput{
				Format: "",
				Params: nil,
			},
			output.FormattedOutput{
				Format: "table",
				Obj: common.ResourceTypeListOutputFormat{
					ResourceType: common.ResourceType{
						Name:                      "MyCompany.Resources/testResources",
						ResourceProviderNamespace: "MyCompany.Resources",
						APIVersions:               map[string]*common.APIVersionProperties{"2023-10-01-preview": {}},
					},
					APIVersionList: []string{"2023-10-01-preview"},
				},
				Options: output.FormatterOptions{
					Columns: []output.Column{
						{
							Heading:  "TYPE",
							JSONPath: "{ .Name }",
						},
						{
							Heading:  "NAMESPACE",
							JSONPath: "{ .ResourceProviderNamespace }",
						},
						{
							Heading:  "DESCRIPTION",
							JSONPath: "{ .Description }",
						},
						{
							Heading:  "APIVERSION",
							JSONPath: "{ .APIVersionList }",
						},
					},
				},
			},
		}
		require.Equal(t, expected, outputSink.Writes, "Mismatch in output sink writes")
	})

	t.Run("Resource provider does not exist", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		expectedResourceType := "testResources"

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNotFoundError)
		require.NoError(t, err)

		var logBuffer bytes.Buffer
		logger := func(format string, args ...any) {
			fmt.Fprintf(&logBuffer, format+"\n", args...)
		}

		runner := &Runner{
			ConnectionFactory:                &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			UCPClientFactory:                 clientFactory,
			Output:                           &output.MockOutput{},
			Workspace:                        &workspaces.Workspace{},
			ResourceProvider:                 resourceProviderData,
			Format:                           "table",
			Logger:                           logger,
			ResourceProviderManifestFilePath: "testdata/valid.yaml",
			ResourceTypeName:                 expectedResourceType,
		}

		_ = runner.Run(context.Background())
		logOutput := logBuffer.String()
		require.Contains(t, logOutput, fmt.Sprintf("Creating resource provider %s", runner.ResourceProvider.Name))
	})
	t.Run("Get Resource provider Internal Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		expectedResourceType := "testResources"

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerInternalError)
		require.NoError(t, err)

		var logBuffer bytes.Buffer
		logger := func(format string, args ...any) {
			fmt.Fprintf(&logBuffer, format+"\n", args...)
		}

		runner := &Runner{
			ConnectionFactory:                &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			UCPClientFactory:                 clientFactory,
			Output:                           &output.MockOutput{},
			Workspace:                        &workspaces.Workspace{},
			ResourceProvider:                 resourceProviderData,
			Format:                           "table",
			Logger:                           logger,
			ResourceProviderManifestFilePath: "testdata/valid.yaml",
			ResourceTypeName:                 expectedResourceType,
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Contains(t, err.Error(), "unexpected status code 500.")
	})
}
