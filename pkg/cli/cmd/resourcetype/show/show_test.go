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

package show

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
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
			Input:         []string{"Applications.Test/exampleResources"},
			ExpectedValid: true,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: not a resource type",
			Input:         []string{"Applications.Test"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: too many arguments",
			Input:         []string{"Applications.Test/exampleResources", "dddd"},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
		{
			Name:          "Invalid: not enough many arguments",
			Input:         []string{},
			ExpectedValid: false,
			ConfigHolder:  framework.ConfigHolder{Config: config},
		},
	}
	radcli.SharedValidateValidation(t, NewCommand, testcases)
}

func Test_Run(t *testing.T) {
	t.Run("Success: Resource Type Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		resourceType := common.ResourceType{
			Name:                      "MyCompany.Resources/testResources",
			ResourceProviderNamespace: "MyCompany.Resources",
			Description:               "Resource type description",
			APIVersions: map[string]*common.APIVersionProperties{"2023-10-01-preview": {
				Schema: map[string]any{
					"properties": map[string]any{
						"application": map[string]any{
							"type":        "string",
							"description": "The name of the application.",
						},
						"environment": map[string]any{
							"type":        "string",
							"description": "The name of the environment.",
						},
						"database": map[string]any{
							"type":        "string",
							"description": "The name of the database.",
							"readOnly":    true,
						},
					},
					"required": []any{
						"environment",
					},
				},
			}},
		}

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
		require.NoError(t, err)

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
			UCPClientFactory:          clientFactory,
			Workspace:                 workspace,
			Format:                    "table",
			Output:                    outputSink,
			ResourceTypeName:          "MyCompany.Resources/testResources",
			ResourceProviderNamespace: "MyCompany.Resources",
			ResourceTypeSuffix:        "testResources",
		}

		err = runner.Run(context.Background())
		require.NoError(t, err)

		expected := []any{
			output.FormattedOutput{
				Format:  "table",
				Obj:     resourceType,
				Options: common.GetResourceTypeShowTableFormat(),
			},
			output.LogOutput{
				Format: "\nDESCRIPTION:",
			},
			output.LogOutput{
				Format: "%s",
				Params: []any{"Resource type description"},
			},
			output.LogOutput{
				Format: "API VERSION: %s\n",
				Params: []any{"2023-10-01-preview"},
			},
			output.LogOutput{
				Format: "TOP-LEVEL PROPERTIES:\n",
			},
			output.FormattedOutput{
				Format:  "table",
				Options: common.GetResourceTypeShowSchemaTableFormat(),
				Obj: []FieldSchema{
					{
						Name:        "application",
						Description: "The name of the application.",
						Type:        "string",
						IsRequired:  false,
						IsReadOnly:  false,
						Properties:  map[string]FieldSchema{},
					},
					{
						Name:        "database",
						Description: "The name of the database.",
						Type:        "string",
						IsRequired:  false,
						IsReadOnly:  true,
						Properties:  map[string]FieldSchema{},
					},
					{
						Name:        "environment",
						Description: "The name of the environment.",
						Type:        "string",
						IsRequired:  true,
						IsReadOnly:  false,
						Properties:  map[string]FieldSchema{},
					},
				},
			},
			output.LogOutput{
				Format: "",
			},
		}
		require.Equal(t, expected, outputSink.Writes)
	})

	t.Run("Error: Resource Provider Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNotFoundError)
		require.NoError(t, err)
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
			UCPClientFactory:          clientFactory,
			Workspace:                 workspace,
			Format:                    "table",
			Output:                    outputSink,
			ResourceTypeName:          "Applications.AnotherTest/exampleResources",
			ResourceProviderNamespace: "Applications.AnotherTest",
			ResourceTypeSuffix:        "exampleResources",
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("The resource type \"Applications.AnotherTest/exampleResources\" does not exist."), err)

		require.Empty(t, outputSink.Writes)
	})

	t.Run("Error: Resource Type Not Found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
		require.NoError(t, err)
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
			UCPClientFactory:          clientFactory,
			Workspace:                 workspace,
			Format:                    "table",
			Output:                    outputSink,
			ResourceTypeName:          "Applications.Test/anotherResources",
			ResourceProviderNamespace: "Applications.Test",
			ResourceTypeSuffix:        "anotherResources",
		}

		err = runner.Run(context.Background())
		require.Error(t, err)
		require.Equal(t, clierrors.Message("The resource type \"Applications.Test/anotherResources\" does not exist."), err)

		require.Empty(t, outputSink.Writes)
	})
}

func Test_GetResourceTypeSchema_Properties(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The resource name.",
			},
			"settings": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"replicas": map[string]any{
						"type":        "integer",
						"description": "Number of replicas.",
					},
				},
				"required": []any{"replicas"},
			},
		},
		"required": []any{"name"},
	}

	result := GetResourceTypeSchema(schema)

	require.Equal(t, map[string]FieldSchema{
		"name": {
			Name:        "name",
			Type:        "string",
			Description: "The resource name.",
			IsRequired:  true,
			IsReadOnly:  false,
			Properties:  map[string]FieldSchema{},
		},
		"settings": {
			Name:        "settings",
			Type:        "object",
			Description: "",
			IsRequired:  false,
			IsReadOnly:  false,
			Properties: map[string]FieldSchema{
				"replicas": {
					Name:        "replicas",
					Type:        "integer",
					Description: "Number of replicas.",
					IsRequired:  true,
					IsReadOnly:  false,
					Properties:  map[string]FieldSchema{},
				},
			},
		},
	}, result)
}

func Test_getNestedSchema(t *testing.T) {
	t.Run("returns input when schema has properties", func(t *testing.T) {
		schema := map[string]any{
			"properties": map[string]any{
				"foo": map[string]any{"type": "string"},
			},
		}

		got := getNestedSchema(schema)
		require.Same(t, &schema, &schema)
		require.Equal(t, schema, got)
	})

	t.Run("descends into additionalProperties when no properties key", func(t *testing.T) {
		nested := map[string]any{
			"properties": map[string]any{
				"value": map[string]any{"type": "string"},
			},
		}
		schema := map[string]any{
			"type":                 "object",
			"additionalProperties": nested,
		}

		got := getNestedSchema(schema)
		require.Equal(t, nested, got)
	})

	t.Run("returns input when neither properties nor additionalProperties match", func(t *testing.T) {
		schema := map[string]any{
			"type": "string",
		}

		got := getNestedSchema(schema)
		require.Equal(t, schema, got)
	})

	t.Run("prefers properties over additionalProperties when both exist", func(t *testing.T) {
		schema := map[string]any{
			"properties": map[string]any{
				"a": map[string]any{"type": "string"},
			},
			"additionalProperties": map[string]any{
				"properties": map[string]any{
					"b": map[string]any{"type": "string"},
				},
			},
		}

		got := getNestedSchema(schema)
		require.Equal(t, schema, got)
	})
}

func Test_GetResourceTypeSchema_AdditionalProperties(t *testing.T) {
	schema := map[string]any{
		"properties": map[string]any{
			"data": map[string]any{
				"type": "object",
				"additionalProperties": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"encoding": map[string]any{
							"type":        "string",
							"description": "Content encoding of the value.",
						},
						"value": map[string]any{
							"type":        "string",
							"description": "The secret value.",
						},
					},
				},
			},
		},
	}

	result := GetResourceTypeSchema(schema)

	require.Equal(t, map[string]FieldSchema{
		"data": {
			Name:        "data",
			Type:        "object",
			Description: "",
			IsRequired:  false,
			IsReadOnly:  false,
			Properties: map[string]FieldSchema{
				"encoding": {
					Name:        "encoding",
					Type:        "string",
					Description: "Content encoding of the value.",
					IsRequired:  false,
					IsReadOnly:  false,
					Properties:  map[string]FieldSchema{},
				},
				"value": {
					Name:        "value",
					Type:        "string",
					Description: "The secret value.",
					IsRequired:  false,
					IsReadOnly:  false,
					Properties:  map[string]FieldSchema{},
				},
			},
		},
	}, result)
}
