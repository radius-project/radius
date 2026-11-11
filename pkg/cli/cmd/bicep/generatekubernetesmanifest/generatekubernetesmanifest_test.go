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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
	"github.com/radius-project/radius/test/radcli"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.yaml.in/yaml/v3"
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
			Name:          "rad bicep generate-kubernetes-manifest - valid with stdout destination file",
			Input:         []string{"app.bicep", "-g", "default", "-d", "-"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "-", runner.DestinationFile)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with name and source-revision",
			Input:         []string{"app.bicep", "-g", "default", "--name", "orders-app", "--source-revision", "abc1234"},
			ExpectedValid: true,
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "orders-app", runner.Name)
				require.Equal(t, "abc1234", runner.SourceRevision)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with bicepparam file",
			Input:         []string{"app.bicep", "-g", "default", "-p", "@app.bicepparam"},
			ExpectedValid: true,
			ConfigureMocks: func(mocks radcli.ValidateMocks) {
				mocks.Bicep.EXPECT().
					Call("build-params", "--stdout", "app.bicepparam").
					Return([]byte(`{"parametersJson":"{\"parameters\":{\"tag\":{\"value\":\"v1.2.3\"}}}"}`), nil).
					Times(1)
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, map[string]map[string]any{
					"tag": {"value": "v1.2.3"},
				}, runner.Parameters)
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

		bicepMock := bicep.NewMockInterface(ctrl)
		bicepMock.EXPECT().
			PrepareTemplate(gomock.Any(), bicepFilePath).
			Return(templateMap, bicep.RadiusExtensionPin{}, nil).
			Times(1)

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicepMock,
			Output:          outputSink,
			FilePath:        bicepFilePath,
			Parameters:      parametersMap,
			FileSystem:      filesystem.NewMemMapFileSystem(),
			DestinationFile: yamlFilePath,
			Group:           resourceGroup,
		}

		fileExists := runner.FileSystem.Exists(yamlFilePath)
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(t.Context())
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

	t.Run("Create DeploymentTemplate with an AWS scope sets the AWS owner", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		const awsScope = "/planes/aws/aws/accounts/123456789012/regions/us-west-2"

		bicepMock := bicep.NewMockInterface(ctrl)
		bicepMock.EXPECT().
			PrepareTemplate(gomock.Any(), "app.bicep").
			Return(map[string]any{}, bicep.RadiusExtensionPin{}, nil).
			Times(1)

		runner := &Runner{
			Bicep:           bicepMock,
			Output:          &output.MockOutput{},
			FilePath:        "app.bicep",
			FileSystem:      filesystem.NewMemMapFileSystem(),
			DestinationFile: "app.yaml",
			Group:           "default",
			AWSScope:        awsScope,
		}

		err := runner.Run(t.Context())
		require.NoError(t, err)

		actual, err := runner.FileSystem.ReadFile("app.yaml")
		require.NoError(t, err)
		var manifest struct {
			Spec struct {
				ProviderConfig string `yaml:"providerConfig"`
			} `yaml:"spec"`
		}
		require.NoError(t, yaml.Unmarshal(actual, &manifest))

		var providerConfig sdkclients.ProviderConfig
		require.NoError(t, json.Unmarshal([]byte(manifest.Spec.ProviderConfig), &providerConfig))
		require.Equal(t, &sdkclients.AWS{
			Type:  sdkclients.ProviderTypeAWS,
			Value: sdkclients.Value{Scope: awsScope, Owner: "deploy;/planes/radius/local/resourceGroups/default;;"},
		}, providerConfig.AWS)
	})

	t.Run("Omits spec.parameters when parameters map is empty", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		bicepMock := bicep.NewMockInterface(ctrl)
		bicepMock.EXPECT().
			PrepareTemplate(gomock.Any(), "app.bicep").
			Return(map[string]any{}, bicep.RadiusExtensionPin{}, nil).
			Times(1)

		runner := &Runner{
			Bicep:           bicepMock,
			Output:          &output.MockOutput{},
			FilePath:        "app.bicep",
			FileSystem:      filesystem.NewMemMapFileSystem(),
			DestinationFile: "app.yaml",
			Group:           "default",
		}

		require.NoError(t, runner.Run(t.Context()))

		actual, err := runner.FileSystem.ReadFile("app.yaml")
		require.NoError(t, err)
		require.NotContains(t, string(actual), "parameters:")

		var manifest map[string]any
		require.NoError(t, yaml.Unmarshal(actual, &manifest))
		spec, ok := manifest["spec"].(map[string]any)
		require.True(t, ok)
		require.NotContains(t, spec, "parameters")
	})

	t.Run("Writes manifest to stdout without log message when destination-file is -", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		bicepMock := bicep.NewMockInterface(ctrl)
		bicepMock.EXPECT().
			PrepareTemplate(gomock.Any(), "app.bicep").
			Return(map[string]any{"resources": map[string]any{}}, bicep.RadiusExtensionPin{}, nil).
			Times(1)

		var stdout bytes.Buffer
		outputSink := &output.MockOutput{}
		memFS := filesystem.NewMemMapFileSystem()
		runner := &Runner{
			Bicep:           bicepMock,
			Output:          outputSink,
			Stdout:          &stdout,
			FilePath:        "app.bicep",
			FileSystem:      memFS,
			DestinationFile: "-",
			Group:           "default",
			Name:            "my-app",
			SourceRevision:  "deadbeef",
		}

		require.NoError(t, runner.Run(t.Context()))
		require.Empty(t, outputSink.Writes)
		require.False(t, memFS.Exists("-"))

		var manifest struct {
			APIVersion string `yaml:"apiVersion"`
			Kind       string `yaml:"kind"`
			Metadata   struct {
				Name        string            `yaml:"name"`
				Annotations map[string]string `yaml:"annotations"`
			} `yaml:"metadata"`
			Spec map[string]any `yaml:"spec"`
		}
		require.NoError(t, yaml.Unmarshal(stdout.Bytes(), &manifest))
		require.Equal(t, "radapp.io/v1alpha3", manifest.APIVersion)
		require.Equal(t, "DeploymentTemplate", manifest.Kind)
		require.Equal(t, "my-app", manifest.Metadata.Name)
		require.Equal(t, map[string]string{"radapp.io/source-revision": "deadbeef"}, manifest.Metadata.Annotations)
		require.NotContains(t, manifest.Spec, "parameters")
	})

	t.Run("Rejects non-string parameter values without panicking", func(t *testing.T) {
		ctrl := gomock.NewController(t)

		bicepMock := bicep.NewMockInterface(ctrl)
		bicepMock.EXPECT().
			PrepareTemplate(gomock.Any(), "app.bicep").
			Return(map[string]any{}, bicep.RadiusExtensionPin{}, nil).
			Times(1)

		var stdout bytes.Buffer
		runner := &Runner{
			Bicep:      bicepMock,
			Output:     &output.MockOutput{},
			Stdout:     &stdout,
			FilePath:   "app.bicep",
			FileSystem: filesystem.NewMemMapFileSystem(),
			Parameters: map[string]map[string]any{
				"tag":      {"value": "v1.0.0"},
				"replicas": {"value": 3},
			},
			DestinationFile: "-",
			Group:           "default",
		}

		err := runner.Run(t.Context())
		require.ErrorContains(t, err, `parameter "replicas" must have a string value, got int`)
	})
}
