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
			Name:          "Valid: no resource type argument",
			Input:         []string{"--from-file", "testdata/valid.yaml"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: resource type not present in manifest",
			Input:         []string{"myResources", "--from-file", "testdata/valid.yaml"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: resource type created when provider exists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
		require.NoError(t, err)

		var logBuffer bytes.Buffer
		logger := func(format string, args ...any) {
			fmt.Fprintf(&logBuffer, format+"\n", args...)
		}

		outputSink := &output.MockOutput{}
		runner := &Runner{
			UCPClientFactory:                 clientFactory,
			Output:                           outputSink,
			Workspace:                        &workspaces.Workspace{},
			ResourceProvider:                 resourceProviderData,
			Format:                           "table",
			Logger:                           logger,
			ResourceProviderManifestFilePath: "testdata/valid.yaml",
			ResourceTypeName:                 "testResources",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err) // Verify the correct log messages are output
		expectedLogs := []interface{}{
			output.LogOutput{
				Format: "Registering resource type %q for resource provider %q.",
				Params: []interface{}{"testResources", "MyCompany.Resources4"},
			},
		}

		for _, expectedLog := range expectedLogs {
			require.Contains(t, outputSink.Writes, expectedLog, "Expected log message not found")
		}

		// Verify RegisterType was called (should see specific log messages)
		logOutput := logBuffer.String()
		require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s with capabilities", runner.ResourceProvider.Name, "testResources"))
	})

	t.Run("No resource type name provided - registers entire manifest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
		require.NoError(t, err)

		var logBuffer bytes.Buffer
		logger := func(format string, args ...any) {
			fmt.Fprintf(&logBuffer, format+"\n", args...)
		}

		outputSink := &output.MockOutput{}
		runner := &Runner{
			UCPClientFactory:                 clientFactory,
			Output:                           outputSink,
			Workspace:                        &workspaces.Workspace{},
			ResourceProvider:                 resourceProviderData,
			Format:                           "table",
			Logger:                           logger,
			ResourceProviderManifestFilePath: "testdata/valid.yaml",
			ResourceTypeName:                 "", // Empty resource type name
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		// Verify the correct log message is output
		expectedLog := output.LogOutput{
			Format: "No resource type name provided. Registering all resource types in the manifest for resource provider %q.",
			Params: []interface{}{"MyCompany.Resources4"},
		}
		require.Contains(t, outputSink.Writes, expectedLog, "Expected log message for no resource type name provided")

		// Verify RegisterResourceProvider was called
		logOutput := logBuffer.String()
		require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s", runner.ResourceProvider.Name, "testResources"))
		require.Contains(t, logOutput, fmt.Sprintf("Creating resource type %s/%s", runner.ResourceProvider.Name, "prodResources"))
	})

	t.Run("Resource provider does not exist - registers resource provider with single type", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceProviderData, err := manifest.ReadFile("testdata/valid.yaml")
		require.NoError(t, err)

		expectedResourceType := "testResources"

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNotFoundError)
		require.NoError(t, err)

		var logBuffer bytes.Buffer
		logger := func(format string, args ...any) {
			fmt.Fprintf(&logBuffer, format+"\n", args...)
		}

		outputSink := &output.MockOutput{}
		runner := &Runner{
			UCPClientFactory:                 clientFactory,
			Output:                           outputSink,
			Workspace:                        &workspaces.Workspace{},
			ResourceProvider:                 resourceProviderData,
			Format:                           "table",
			Logger:                           logger,
			ResourceProviderManifestFilePath: "testdata/valid.yaml",
			ResourceTypeName:                 expectedResourceType,
		}

		_ = runner.Run(context.Background())

		// Verify the correct log messages are output
		expectedLogs := []interface{}{
			output.LogOutput{
				Format: "Registering resource type %q for resource provider %q.",
				Params: []interface{}{"testResources", "MyCompany.Resources4"},
			},
		}

		// Verify no other ersource types are registered
		shouldNotContain := []interface{}{
			output.LogOutput{
				Format: "Registering resource type %q for resource provider %q.",
				Params: []interface{}{"prodResources", "MyCompany.Resources4"},
			},
		}

		for _, expectedLog := range expectedLogs {
			require.Contains(t, outputSink.Writes, expectedLog, "Expected log message not found")
			require.NotContains(t, outputSink.Writes, shouldNotContain, "Log messages related to unspecified resource types should not be present")
		}
	})
	t.Run("Get Resource provider Internal Error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

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
