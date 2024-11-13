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
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)

	resource := map[string]any{
		"properties": map[string]any{
			"message": "Hello, world!",
		},
	}
	b, err := json.Marshal(resource)
	require.NoError(t, err)

	directory := t.TempDir()
	err = os.WriteFile(filepath.Join(directory, "valid-resource.json"), b, 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(directory, "invalid-resource.json"), []byte("{askdfe}"), 0644)
	require.NoError(t, err)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid: JSON file",
			Input:         []string{"Applications.Test/exampleResources", "my-example", "--from-file", filepath.Join(directory, "valid-resource.json")},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:                "Invalid: non-JSON file",
			Input:               []string{"Applications.Test/exampleResources", "my-example", "--from-file", filepath.Join(directory, "invalid-resource.json")},
			ExpectedValid:       false,
			ConfigHolder:        framework.ConfigHolder{Config: config},
			CreateTempDirectory: directory,
		},
		{
			Name:          "Invalid: missing input file",
			Input:         []string{"Applications.Test/exampleResources", "my-example"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many arguments",
			Input:         []string{"Applications.Test/exampleResources", "my-example", "@" + filepath.Join(directory, "valid-resource.json"), "dddddd"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: resource provider created", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		expectedResource := &generated.GenericResource{
			Properties: map[string]any{
				"message": "Hello, world!",
			},
		}

		appManagementClient := clients.NewMockApplicationsManagementClient(ctrl)
		appManagementClient.EXPECT().
			CreateOrUpdateResource(gomock.Any(), "Applications.Test/exampleResources", "my-example", expectedResource).
			Return(*expectedResource, nil).
			Times(1)

		outputSink := &output.MockOutput{}

		runner := &Runner{
			ConnectionFactory: &connections.MockFactory{ApplicationsManagementClient: appManagementClient},
			Output:            outputSink,
			Workspace:         &workspaces.Workspace{},
			ResourceType:      "Applications.Test/exampleResources",
			ResourceName:      "my-example",
			Resource:          expectedResource,
			Format:            "table",
		}

		err := runner.Run(context.Background())
		require.NoError(t, err)
	})
}
