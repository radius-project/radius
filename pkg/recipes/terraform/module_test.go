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
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/stretchr/testify/require"
)

func Test_InspectTFModuleConfig(t *testing.T) {
	tests := []struct {
		name       string
		recipe     *recipes.EnvironmentDefinition
		workingDir string
		result     *moduleInspectResult
		err        string
		errExact   bool
		setup      func(t *testing.T) string
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
						Source:               "hashicorp/aws",
						Version:              ">=3.0",
						ConfigurationAliases: []string{"aws.eu-west-1", "aws.eu-west-2"},
					},
				},
				ResultOutputExists: false,
				Parameters:         map[string]any{},
			},
		},
		{
			name: "aws provider - partial information",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-module-providerpartialinfo",
				TemplatePath: "test-module-providerpartialinfo",
			},
			workingDir: "testdata",
			result: &moduleInspectResult{
				ContextVarExists: false,
				RequiredProviders: map[string]*config.RequiredProviderInfo{
					"aws": {
						Source: "hashicorp/aws",
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
			err:      "The Terraform configuration in location invalid-module is not found.",
			errExact: true,
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
		{
			name:       "missing submodule path",
			workingDir: "testdata",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-submodule",
				TemplatePath: "test-submodule//missing-submodule",
			},
			err:      "The Terraform configuration in location test-submodule//missing-submodule is not found.",
			errExact: true,
		},
		{
			name:       "module name outside module root",
			workingDir: "testdata",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "../test-submodule",
				TemplatePath: "test-submodule",
			},
			err: "module path \"../test-submodule\" must be local",
		},
		{
			name:       "submodule path outside module root",
			workingDir: "testdata",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-submodule",
				TemplatePath: "test-submodule//../missing-submodule",
			},
			err: "module path \"../missing-submodule\" must be local",
		},
		{
			name: "module directory without Terraform configuration",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-module-no-config",
				TemplatePath: "test-module-no-config",
			},
			err:      "The Terraform configuration in location test-module-no-config is not found.",
			errExact: true,
			setup: func(t *testing.T) string {
				t.Helper()
				workingDir := t.TempDir()
				moduleDir := filepath.Join(workingDir, moduleRootDir, "test-module-no-config")
				require.NoError(t, os.MkdirAll(moduleDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "README.md"), []byte("no terraform configuration"), 0644))
				return workingDir
			},
		},
		{
			name: "module directory with Terraform-named subdirectory",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-module-dir-config-like-name",
				TemplatePath: "test-module-dir-config-like-name",
			},
			err:      "The Terraform configuration in location test-module-dir-config-like-name is not found.",
			errExact: true,
			setup: func(t *testing.T) string {
				t.Helper()
				workingDir := t.TempDir()
				moduleDir := filepath.Join(workingDir, moduleRootDir, "test-module-dir-config-like-name")
				require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "main.tf"), 0755))
				require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "README.md"), []byte("no terraform configuration"), 0644))
				return workingDir
			},
		},
		{
			name: "invalid Terraform configuration returns load module error",
			recipe: &recipes.EnvironmentDefinition{
				Name:         "test-module-invalid-config",
				TemplatePath: "test-module-invalid-config",
			},
			err: "error loading the module",
			setup: func(t *testing.T) string {
				t.Helper()
				workingDir := t.TempDir()
				moduleDir := filepath.Join(workingDir, moduleRootDir, "test-module-invalid-config")
				require.NoError(t, os.MkdirAll(moduleDir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "main.tf"), []byte("terraform {"), 0644))
				return workingDir
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			workingDir := tc.workingDir
			if tc.setup != nil {
				workingDir = tc.setup(t)
			}

			result, err := inspectModule(workingDir, tc.recipe)
			if tc.err != "" {
				require.Error(t, err)
				if tc.errExact {
					require.EqualError(t, err, tc.err)
				} else {
					require.Contains(t, err.Error(), tc.err)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.result, result)
		})
	}
}
