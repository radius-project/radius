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

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/terraform/config"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestCreateWorkingDir_Created(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	expectedWorkingDir := filepath.Join(testDir, executionSubDir)
	workingDir, err := createWorkingDir(testcontext.New(t), testDir)
	require.NoError(t, err)
	require.Equal(t, expectedWorkingDir, workingDir)

	// Assert that the working directory was created.
	_, err = os.Stat(workingDir)
	require.NoError(t, err)
}

func TestCreateWorkingDir_Error(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	// Create a read-only directory within the temporary directory.
	readOnlyDir := filepath.Join(testDir, "read-only-dir")
	err := os.MkdirAll(readOnlyDir, 0555)
	require.NoError(t, err)

	// Call createWorkingDir with the read-only directory.
	_, err = createWorkingDir(testcontext.New(t), readOnlyDir)

	// Assert that createWorkingDir returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create working directory")
}

func TestInitAndApply_EmptyWorkingDirPath(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")

	_, err := initAndApply(testcontext.New(t), "", execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Terraform cannot be initialised with empty workdir")
}

func TestGeneratedConfig(t *testing.T) {
	configTests := []struct {
		name       string
		workingDir string
		opts       Options
		err        string
	}{
		{
			name: "empty recipe name",
			opts: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{
					TemplatePath: "test/module/source",
				},
			},
			err: ErrRecipeNameEmpty.Error(),
		}, {
			name:       "invalid working dir",
			workingDir: "/invalid-dir",
			opts: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{
					Name:         "test-recipe",
					TemplatePath: "test/module/source",
				},
				ResourceRecipe: &recipes.ResourceMetadata{},
			},
			err: "error creating file: open /invalid-dir/main.tf.json",
		}, {
			name: "invalid exec path",
			opts: Options{
				EnvRecipe: &recipes.EnvironmentDefinition{
					Name:         "test-recipe",
					TemplatePath: "test/module/source",
				},
				ResourceRecipe: &recipes.ResourceMetadata{},
			},
			err: "/terraform: no such file or directory",
		},
	}

	for _, tc := range configTests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := testcontext.New(t)
			if tc.workingDir == "" {
				tc.workingDir = t.TempDir()
			}
			execPath := filepath.Join(tc.workingDir, "terraform")
			e := executor{}
			_, err := e.generateConfig(ctx, tc.workingDir, execPath, tc.opts)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.err)
		})
	}
}

func Test_GetTerraformConfig(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	workingDir, err := createWorkingDir(testcontext.New(t), testDir)
	require.NoError(t, err)
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
	tfConfig, err := getTerraformConfig(testcontext.New(t), workingDir, options)
	require.NoError(t, err)
	require.Equal(t, &expectedConfig, tfConfig)
}
