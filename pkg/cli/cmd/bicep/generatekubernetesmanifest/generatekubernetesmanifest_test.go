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

package bicep

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	testcases := []radcli.ValidateInput{
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with group short flag",
			Input:         []string{"app.bicep", "-g", "default"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "default", runner.Group)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with group long flag",
			Input:         []string{"app.bicep", "--group", "default"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "default", runner.Group)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - invalid with no group provided",
			Input:         []string{"app.bicep"},
			ExpectedValid: false,
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with parameters",
			Input:         []string{"app.bicep", "-g", "default", "-p", "foo=bar", "--parameters", "a=b", "--parameters", "@testdata/parameters.json"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				expectedParameters := map[string]map[string]any{
					"foo": {
						"value": "bar",
					},
					"a": {
						"value": "b",
					},
					"b": {
						"value": "c",
					},
				}
				require.Equal(t, expectedParameters, runner.Parameters)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - invalid parameter format",
			Input:         []string{"app.bicep", "-g", "default", "--parameters", "invalid-format"},
			ExpectedValid: false,
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - missing file argument",
			Input:         []string{},
			ExpectedValid: false,
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - too many args",
			Input:         []string{"app.bicep", "-g", "default", "anotherfile.bicep"},
			ExpectedValid: false,
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with destination file long flag",
			Input:         []string{"app.bicep", "-g", "default", "--destination-file", "test.yaml"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "test.yaml", runner.DestinationFile)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with destination file short flag",
			Input:         []string{"app.bicep", "-g", "default", "-d", "test.yaml"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "test.yaml", runner.DestinationFile)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - invalid destination file",
			Input:         []string{"app.bicep", "-g", "default", "--destination-file", "test.json"},
			ExpectedValid: false,
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with azure scope",
			Input:         []string{"app.bicep", "-g", "default", "--azure-scope", "azure-scope-value"},
			ExpectedValid: true,
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with aws scope",
			Input:         []string{"app.bicep", "-g", "default", "--aws-scope", "aws-scope-value"},
			ExpectedValid: true,
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Create DeploymentTemplate", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceGroup := "default"
		testName := "deploymenttemplate"
		bicepFilePath := fmt.Sprintf("%s.bicep", testName)
		parametersFilePath := fmt.Sprintf("%s-parameters.json", testName)
		jsonFilePath := fmt.Sprintf("%s.json", testName)
		yamlFilePath := fmt.Sprintf("%s.yaml", testName)

		template, err := os.ReadFile(filepath.Join("testdata", testName, jsonFilePath))
		require.NoError(t, err)

		var templateMap map[string]any
		err = json.Unmarshal([]byte(template), &templateMap)
		require.NoError(t, err)

		parameters, err := os.ReadFile(filepath.Join("testdata", testName, parametersFilePath))
		require.NoError(t, err)

		var parametersMap map[string]map[string]any
		err = json.Unmarshal([]byte(parameters), &parametersMap)
		require.NoError(t, err)

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate(bicepFilePath).
			Return(templateMap, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Output:          outputSink,
			FilePath:        bicepFilePath,
			Parameters:      parametersMap,
			FileSystem:      filesystem.NewMemMapFileSystem(nil),
			DestinationFile: yamlFilePath,
			Group:           resourceGroup,
		}

		fileExists := runner.FileSystem.Exists(yamlFilePath)
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists = runner.FileSystem.Exists(yamlFilePath)
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, yamlFilePath, runner.DestinationFile)

		expected, err := os.ReadFile(filepath.Join("testdata", testName, yamlFilePath))
		require.NoError(t, err)

		actual, err := runner.FileSystem.ReadFile(yamlFilePath)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})
}
