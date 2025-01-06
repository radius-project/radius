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

	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/test/radcli"
	"github.com/stretchr/testify/require"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	config := radcli.LoadConfigWithWorkspace(t)

	testcases := []radcli.ValidateInput{
		{
			Name:          "Valid",
			Input:         []string{"coolResources", "--from-file", "testdata/valid.yaml"},
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

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		expectedResourceType := "testResources"

		clientFactory, err := manifest.NewTestClientFactory()
		require.NoError(t, err)

		var logBuffer bytes.Buffer
		logger := func(format string, args ...any) {
			fmt.Fprintf(&logBuffer, format+"\n", args...)
		}

		runner := &Runner{
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
		require.NoError(t, err)

		logOutput := logBuffer.String()
		require.Contains(t, logOutput, fmt.Sprintf("Resource type %s/%s created successfully", resourceProviderData.Name, expectedResourceType))
	})
}
