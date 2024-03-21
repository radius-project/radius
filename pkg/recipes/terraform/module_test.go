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

package terraform

import (
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/stretchr/testify/require"
)

func Test_InspectTFModuleConfig(t *testing.T) {
	tests := []struct {
		name         string
		recipe       *recipes.EnvironmentDefinition
		workingDir   string
		moduleName   string
		templatePath string
		result       *moduleInspectResult
		err          string
	}{
		{
			name: "aws provider only",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-module-provideronly",
				TemplatePath: "test-module-provideronly",
			},
			workingDir: "testdata",
			result: &moduleInspectResult{
				ContextVarExists: false,
				RequiredProviders: map[string]*config.RequiredProviderInfo{
					"aws": {
						Source:  "hashicorp/aws",
						Version: ">=3.0",
					},
				},
				ResultOutputExists: false,
				Parameters:         map[string]any{},
			},
		},
		{
			name:       "aws provider with recipe context variable, output and parameters",
			workingDir: "testdata",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-module-recipe-context-outputs",
				TemplatePath: "test-module-recipe-context-outputs",
			},
			result: &moduleInspectResult{
				ContextVarExists: true,
				RequiredProviders: map[string]*config.RequiredProviderInfo{
					"aws": {
						Source:  "hashicorp/aws",
						Version: ">=3.0",
					},
				},
				ResultOutputExists: true,
				Parameters: map[string]any{
					"context": map[string]any{
						"name":         "context",
						"type":         "object({\n    resource = object({\n      name = string\n      id = string\n      type = string\n    })\n\n    application = object({\n      name = string\n      id = string\n    })\n\n    environment = object({\n      name = string\n      id = string\n    })\n\n    runtime = object({\n      kubernetes = optional(object({\n        namespace = string\n        environmentNamespace = string\n      }))\n    })\n\n    azure = optional(object({\n      resourceGroup = object({\n        name = string\n        id = string\n      })\n      subscription = object({\n        subscriptionId = string\n        id = string\n      })\n    }))\n    \n    aws = optional(object({\n      region = string\n      account = string\n    }))\n  })",
						"description":  "This variable contains Radius recipe context.",
						"defaultValue": nil,
						"required":     true,
						"sensitive":    false,
						"pos": tfconfig.SourcePos{
							Filename: "testdata/.terraform/modules/test-module-recipe-context-outputs/variables.tf",
							Line:     1,
						},
					},
				},
			},
		},
		{
			name:       "invalid module name - non existent module directory",
			workingDir: "testdata",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "invalid-module",
				TemplatePath: "invalid-module",
			},
			err: "error loading the module",
		},
		{
			name:       "submodule path",
			workingDir: "testdata",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-submodule",
				TemplatePath: "test-submodule//submodule",
			},
			result: &moduleInspectResult{
				ContextVarExists: false,
				RequiredProviders: map[string]*config.RequiredProviderInfo{
					"aws": {
						Source:  "hashicorp/aws",
						Version: ">=3.0",
					},
				},
				ResultOutputExists: false,
				Parameters:         map[string]any{},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := inspectModule(tc.workingDir, tc.recipe)
			if tc.err != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.result, result)
		})
	}
}
