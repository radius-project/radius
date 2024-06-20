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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
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

	resourceProviderData, err := os.ReadFile("testdata/resourceprovider.json")
	require.NoError(t, err)

	directory := t.TempDir()
	err = os.WriteFile(filepath.Join(directory, "valid-resourceprovider.json"), resourceProviderData, 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(directory, "invalid-resourceprovider.json"), []byte("{askdfe}"), 0644)
	require.NoError(t, err)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid: inline JSON",
			Input:         []string{"Applications.Test", string(resourceProviderData)},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:                "Valid: JSON file",
			Input:               []string{"Applications.Test", "@valid-resourceprovider.json"},
			ExpectedValid:       true,
			ConfigHolder:        framework.ConfigHolder{Config: config},
			CreateTempDirectory: directory,
		},
		{
			Name:          "Valid: inline non-JSON",
			Input:         []string{"Applications.Test", "{askdfe}"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:                "Invalid: non-JSON file",
			Input:               []string{"Applications.Test", "@invalid-resourceprovider.json"},
			ExpectedValid:       false,
			ConfigHolder:        framework.ConfigHolder{Config: config},
			CreateTempDirectory: directory,
		},
		{
			Name:          "Invalid: missing arguments",
			Input:         []string{"Applications.Test", "@valid-resourceprovider.json"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many arguments",
			Input:         []string{"Applications.Test", "@valid-resourceprovider.json", "dddddd"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: resource created", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		resourceProviderData, err := os.ReadFile("testdata/resourceprovider.json")
		require.NoError(t, err)

		expectedResourceProvider := &v20231001preview.ResourceProviderResource{}
		err = json.Unmarshal(resourceProviderData, expectedResourceProvider)
		require.NoError(t, err)

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			CreateOrUpdateResourceProvider(gomock.Any(), "local", "Applications.Test", expectedResourceProvider).
			Return(*expectedResourceProvider, nil).
			Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory:         &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:                    outputSink,
			Workspace:                 &workspaces.Workspace{},
			ResourceProviderNamespace: "Applications.Test",
			ResourceProvider:          expectedResourceProvider,
			Format:                    "table",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)
	})
}
