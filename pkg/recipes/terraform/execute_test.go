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
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	dm "github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestGenerateConfig(t *testing.T) {
	configTests := []struct {
		name       string
		workingDir string
		opts       Options
		err        string
	}{
		{
			name: "empty recipe name error",
			opts: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{
					TemplatePath: "test/module/source",
				},
			},
			err: ErrRecipeNameEmpty.Error(),
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			if tc.workingDir == "" {
				tc.workingDir = t.TempDir()
			}
			tf, err := tfexec.NewTerraform(tc.workingDir, filepath.Join(tc.workingDir, "terraform"))
			require.NoError(t, err)

			e := executor{}
			_, err = e.generateConfig(ctx, tf, tc.opts)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func Test_GetTerraformConfig(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	expectedConfig := config.TerraformConfig{
		Module: map[string]config.TFModuleConfig{
			"test-recipe": {"source": "test/module/source"}},
	}
	tfConfig, err := getTerraformConfig(testcontext.New(t), testDir, options)
	require.NoError(t, err)
	require.Equal(t, &expectedConfig, tfConfig)
}

func Test_GetTerraformConfig_EmptyRecipeName(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	_, err := getTerraformConfig(testcontext.New(t), testDir, options)
	require.Error(t, err)
	require.Equal(t, err, ErrRecipeNameEmpty)
}

func Test_GetTerraformConfig_InvalidDirectory(t *testing.T) {
	workingDir := "invalid-directory"
	options := Options{
		EnvRecipe: &recipes.EnvironmentDefinition{
			Name:         "test-recipe",
			TemplatePath: "test/module/source",
		},
		ResourceRecipe: &recipes.ResourceMetadata{},
	}

	_, err := getTerraformConfig(testcontext.New(t), workingDir, options)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error creating file: open invalid-directory/main.tf.json: no such file or directory")
}

func TestSetEnvironmentVariables(t *testing.T) {
	testCase := []struct {
		name string
		opts Options
	}{
		{
			name: "set environment variables",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Env: dm.EnvironmentVariables{
							AdditionalProperties: map[string]string{
								"TEST_ENV_VAR1": "value1",
								"TEST_ENV_VAR2": "value2",
							},
						},
					},
				},
			},
		},
		{
			name: "AdditionalProperties set to nil",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{
						Env: dm.EnvironmentVariables{
							AdditionalProperties: nil,
						},
					},
				},
			},
		},
		{
			name: "no environment variables",
			opts: Options{
				EnvConfig: &recipes.Configuration{
					RecipeConfig: dm.RecipeConfigProperties{},
				},
			},
		},
	}
	for _, tc := range testCase {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			workingDir := t.TempDir()

			tf, err := tfexec.NewTerraform(workingDir, filepath.Join(workingDir, "terraform"))
			require.NoError(t, err)

			// Create an executor
			e := executor{}

			// Call the function to set environment variables
			err = e.setEnvironmentVariables(ctx, tf, tc.opts.EnvConfig)
			require.NoError(t, err)
		})
	}
}
