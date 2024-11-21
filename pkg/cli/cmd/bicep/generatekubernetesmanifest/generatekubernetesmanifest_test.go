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
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/test/radcli"
	"github.com/spf13/afero"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_CommandValidation(t *testing.T) {
	radcli.SharedCommandValidation(t, NewCommand)
}

func Test_Validate(t *testing.T) {
	configWithWorkspace := radcli.LoadConfigWithWorkspace(t)
	testcases := []radcli.ValidateInput{
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid",
			Input:         []string{"app.bicep"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "/planes/radius/local/resourceGroups/default", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with parameters",
			Input:         []string{"app.bicep", "-p", "foo=bar", "--parameters", "a=b"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "/planes/radius/local/resourceGroups/default", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with parameters file",
			Input:         []string{"app.bicep", "--parameters", "@testdata/parameters.json"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "/planes/radius/local/resourceGroups/default", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - invalid parameter format",
			Input:         []string{"app.bicep", "--parameters", "invalid-format"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - too many args",
			Input:         []string{"app.bicep", "anotherfile.bicep"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with outfile",
			Input:         []string{"app.bicep", "--outfile", "test.yaml"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "/planes/radius/local/resourceGroups/default", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - invalid outfile",
			Input:         []string{"app.bicep", "test.json"},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with azure scope",
			Input:         []string{"app.bicep", "--azure-scope", "azure-scope-value"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "azure-scope-value", runner.AzureScope)
				require.Equal(t, "/planes/radius/local/resourceGroups/default", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with aws scope",
			Input:         []string{"app.bicep", "--aws-scope", "aws-scope-value"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "aws-scope-value", runner.AWSScope)
				require.Equal(t, "/planes/radius/local/resourceGroups/default", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - valid with deployment scope",
			Input:         []string{"app.bicep", "--deployment-scope", "deployment-scope-value"},
			ExpectedValid: true,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         configWithWorkspace,
			},
			ValidateCallback: func(t *testing.T, r framework.Runner) {
				runner := r.(*Runner)
				require.Equal(t, "deployment-scope-value", runner.DeploymentScope)
			},
		},
		{
			Name:          "rad bicep generate-kubernetes-manifest - missing file argument",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder: framework.ConfigHolder{
				ConfigFilePath: "",
				Config:         radcli.LoadEmptyConfig(t),
			},
		},
	}

	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Create DeploymentTemplate (basic)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		template := `
		{
      "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
      "contentVersion": "1.0.0.0",
      "imports": {
        "Radius": {
          "provider": "Radius",
          "version": "latest"
        }
      },
      "languageVersion": "2.1-experimental",
      "metadata": {
        "_EXPERIMENTAL_FEATURES_ENABLED": [
          "Extensibility"
        ],
        "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
        "_generator": {
          "name": "bicep",
          "templateHash": "10886769892319697000",
          "version": "0.30.23.60470"
        }
      },
      "resources": {
        "basic": {
          "import": "Radius",
          "properties": {
            "name": "basic",
            "properties": {
              "compute": {
                "kind": "kubernetes",
                "namespace": "default",
                "resourceId": "self"
              },
              "recipes": {
                "Applications.Datastores/redisCaches": {
                  "default": {
                    "templateKind": "bicep",
                    "templatePath": "ghcr.io/radius-project/recipes/local-dev/rediscaches:latest"
                  }
                }
              }
            }
          },
          "type": "Applications.Core/environments@2023-10-01-preview"
        }
      }
    }
		`

		var templateMap map[string]any
		err := json.Unmarshal([]byte(template), &templateMap)
		require.NoError(t, err)

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("basic.bicep").
			Return(templateMap, nil).
			Times(1)

		filePath := "basic.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Output:          outputSink,
			FilePath:        filePath,
			Parameters:      map[string]map[string]any{},
			FileSystem:      afero.NewMemMapFs(),
			DeploymentScope: "/planes/radius/local/resourceGroups/default",
		}

		fileExists, err := afero.Exists(runner.FileSystem, "basic.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "basic.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "basic.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "basic", "basic.yaml"))
		require.NoError(t, err)

		actual, err := afero.ReadFile(runner.FileSystem, "basic.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate (parameters)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		template := `
    {
      "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
      "contentVersion": "1.0.0.0",
      "imports": {
        "Radius": {
          "provider": "Radius",
          "version": "latest"
        }
      },
      "languageVersion": "2.1-experimental",
      "metadata": {
        "_EXPERIMENTAL_FEATURES_ENABLED": [
          "Extensibility"
        ],
        "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
        "_generator": {
          "name": "bicep",
          "templateHash": "289770176196104222",
          "version": "0.30.23.60470"
        }
      },
      "parameters": {
        "kubernetesNamespace": {
          "defaultValue": "default",
          "type": "string"
        },
        "tag": {
          "defaultValue": "latest",
          "type": "string"
        }
      },
      "resources": {
        "parameters": {
          "import": "Radius",
          "properties": {
            "name": "parameters",
            "properties": {
              "compute": {
                "kind": "kubernetes",
                "namespace": "[parameters('kubernetesNamespace')]",
                "resourceId": "self"
              },
              "recipes": {
                "Applications.Datastores/redisCaches": {
                  "default": {
                    "templateKind": "bicep",
                    "templatePath": "[format('ghcr.io/radius-project/recipes/local-dev/rediscaches:{0}', parameters('tag'))]"
                  }
                }
              }
            }
          },
          "type": "Applications.Core/environments@2023-10-01-preview"
        }
      }
    }
		`

		var templateMap map[string]any
		err := json.Unmarshal([]byte(template), &templateMap)
		require.NoError(t, err)

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("parameters.bicep").
			Return(templateMap, nil).
			Times(1)

		filePath := "parameters.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Output:          outputSink,
			FilePath:        filePath,
			Parameters:      map[string]map[string]any{},
			FileSystem:      afero.NewMemMapFs(),
			DeploymentScope: "/planes/radius/local/resourceGroups/default",
		}

		fileExists, err := afero.Exists(runner.FileSystem, "parameters.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "parameters.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "parameters.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "parameters", "parameters.yaml"))
		require.NoError(t, err)

		actual, err := afero.ReadFile(runner.FileSystem, "parameters.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate (aws)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		template := `
    {
      "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
      "contentVersion": "1.0.0.0",
      "imports": {
        "AWS": {
          "provider": "AWS",
          "version": "latest"
        },
        "Radius": {
          "provider": "Radius",
          "version": "latest"
        }
      },
      "languageVersion": "2.1-experimental",
      "metadata": {
        "_EXPERIMENTAL_FEATURES_ENABLED": [
          "Extensibility"
        ],
        "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
        "_generator": {
          "name": "bicep",
          "templateHash": "4336724644513409792",
          "version": "0.30.23.60470"
        }
      },
      "parameters": {
        "bucketName": {
          "defaultValue": "gkm-bucket",
          "type": "string"
        }
      },
      "resources": {
        "bucket": {
          "import": "AWS",
          "properties": {
            "alias": "[parameters('bucketName')]",
            "properties": {
              "BucketName": "[parameters('bucketName')]"
            }
          },
          "type": "AWS.S3/Bucket@default"
        }
      }
    }
		`

		var templateMap map[string]any
		err := json.Unmarshal([]byte(template), &templateMap)
		require.NoError(t, err)

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("aws.bicep").
			Return(templateMap, nil).
			Times(1)

		filePath := "aws.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Output:          outputSink,
			FilePath:        filePath,
			Parameters:      map[string]map[string]any{},
			FileSystem:      afero.NewMemMapFs(),
			AWSScope:        "awsscope",
			DeploymentScope: "/planes/radius/local/resourceGroups/default",
		}

		fileExists, err := afero.Exists(runner.FileSystem, "aws.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "aws.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "aws.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "aws", "aws.yaml"))
		require.NoError(t, err)

		actual, err := afero.ReadFile(runner.FileSystem, "aws.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate (azure)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		template := `
    {
      "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
      "contentVersion": "1.0.0.0",
      "languageVersion": "2.1-experimental",
      "metadata": {
        "_EXPERIMENTAL_FEATURES_ENABLED": [
          "Extensibility"
        ],
        "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
        "_generator": {
          "name": "bicep",
          "templateHash": "14111843528652336728",
          "version": "0.30.23.60470"
        }
      },
      "resources": {
        "storageAccount": {
          "apiVersion": "2021-04-01",
          "kind": "StorageV2",
          "location": "eastus",
          "name": "gkmstorageaccount",
          "properties": {},
          "sku": {
            "name": "Standard_LRS"
          },
          "type": "Microsoft.Storage/storageAccounts"
        }
      }
    }
		`

		var templateMap map[string]any
		err := json.Unmarshal([]byte(template), &templateMap)
		require.NoError(t, err)

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("azure.bicep").
			Return(templateMap, nil).
			Times(1)

		filePath := "azure.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Output:          outputSink,
			FilePath:        filePath,
			Parameters:      map[string]map[string]any{},
			FileSystem:      afero.NewMemMapFs(),
			AzureScope:      "azurescope",
			DeploymentScope: "/planes/radius/local/resourceGroups/default",
		}

		fileExists, err := afero.Exists(runner.FileSystem, "azure.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "azure.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "azure.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "azure", "azure.yaml"))
		require.NoError(t, err)

		actual, err := afero.ReadFile(runner.FileSystem, "azure.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})

	t.Run("Create DeploymentTemplate (module)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		template := `
    {
      "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
      "contentVersion": "1.0.0.0",
      "languageVersion": "2.1-experimental",
      "metadata": {
        "_EXPERIMENTAL_FEATURES_ENABLED": [
          "Extensibility"
        ],
        "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
        "_generator": {
          "name": "bicep",
          "templateHash": "1040374933922883026",
          "version": "0.30.23.60470"
        }
      },
      "outputs": {
        "storageAccountId": {
          "type": "string",
          "value": "[reference('storageModule').outputs.storageAccountId.value]"
        }
      },
      "parameters": {
        "location": {
          "defaultValue": "[resourceGroup().location]",
          "type": "string"
        },
        "storageAccountName": {
          "type": "string"
        }
      },
      "resources": {
        "storageModule": {
          "apiVersion": "2022-09-01",
          "name": "storageModule",
          "properties": {
            "expressionEvaluationOptions": {
              "scope": "inner"
            },
            "mode": "Incremental",
            "parameters": {
              "location": {
                "value": "[parameters('location')]"
              },
              "storageAccountName": {
                "value": "[parameters('storageAccountName')]"
              }
            },
            "template": {
              "$schema": "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#",
              "contentVersion": "1.0.0.0",
              "languageVersion": "2.1-experimental",
              "metadata": {
                "_EXPERIMENTAL_FEATURES_ENABLED": [
                  "Extensibility"
                ],
                "_EXPERIMENTAL_WARNING": "This template uses ARM features that are experimental. Experimental features should be enabled for testing purposes only, as there are no guarantees about the quality or stability of these features. Do not enable these settings for any production usage, or your production environment may be subject to breaking.",
                "_generator": {
                  "name": "bicep",
                  "templateHash": "17553429517046312167",
                  "version": "0.30.23.60470"
                }
              },
              "outputs": {
                "storageAccountId": {
                  "type": "string",
                  "value": "[resourceId('Microsoft.Storage/storageAccounts', parameters('storageAccountName'))]"
                }
              },
              "parameters": {
                "location": {
                  "type": "string"
                },
                "storageAccountName": {
                  "type": "string"
                }
              },
              "resources": {
                "storageAccount": {
                  "apiVersion": "2021-04-01",
                  "kind": "StorageV2",
                  "location": "[parameters('location')]",
                  "name": "[parameters('storageAccountName')]",
                  "properties": {},
                  "sku": {
                    "name": "Standard_LRS"
                  },
                  "type": "Microsoft.Storage/storageAccounts"
                }
              }
            }
          },
          "type": "Microsoft.Resources/deployments"
        }
      }
    }
		`

		var templateMap map[string]any
		err := json.Unmarshal([]byte(template), &templateMap)
		require.NoError(t, err)

		bicep := bicep.NewMockInterface(ctrl)
		bicep.EXPECT().
			PrepareTemplate("module.bicep").
			Return(templateMap, nil).
			Times(1)

		filePath := "module.bicep"

		outputSink := &output.MockOutput{}
		runner := &Runner{
			Bicep:           bicep,
			Output:          outputSink,
			FilePath:        filePath,
			Parameters:      map[string]map[string]any{},
			FileSystem:      afero.NewMemMapFs(),
			DeploymentScope: "/planes/radius/local/resourceGroups/default",
		}

		fileExists, err := afero.Exists(runner.FileSystem, "module.yaml")
		require.NoError(t, err)
		require.False(t, fileExists)

		err = runner.Run(context.Background())
		require.NoError(t, err)

		fileExists, err = afero.Exists(runner.FileSystem, "module.yaml")
		require.NoError(t, err)
		require.True(t, fileExists)

		require.Equal(t, "module.yaml", runner.OutFile)

		expected, err := os.ReadFile(filepath.Join("testdata", "module", "module.yaml"))
		require.NoError(t, err)

		actual, err := afero.ReadFile(runner.FileSystem, "module.yaml")
		require.NoError(t, err)
		require.Equal(t, string(expected), string(actual))
	})
}
