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

	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestInspectTFModuleConfig(t *testing.T) {
	inspectTests := []struct {
		name       string
		workingDir string
		moduleName string
		result     *moduleInspectResult
		err        string
	}{
		{
			name:       "aws provider only",
			workingDir: "testdata",
			moduleName: "test-module-provideronly",
			result: &moduleInspectResult{
				ContextExists:     false,
				RequiredProviders: []string{"aws"},
			},
		}, {
			name:       "aws provider with recipecontext",
			workingDir: "testdata",
			moduleName: "test-module-recipe-context",
			result: &moduleInspectResult{
				ContextExists:     true,
				RequiredProviders: []string{"aws"},
			},
		}, {
			name:       "invalid module name",
			workingDir: "testdata",
			moduleName: "invalid-module",
			err:        "error loading the module",
		},
	}

	for _, tc := range inspectTests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := inspectTFModuleConfig(tc.workingDir, tc.moduleName)
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

func TestDownloadModule_EmptyWorkingDirPath_Error(t *testing.T) {
	// Create a temporary test directory.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")

	err := downloadModule(testcontext.New(t), "", execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Terraform cannot be initialised with empty workdir")
}
