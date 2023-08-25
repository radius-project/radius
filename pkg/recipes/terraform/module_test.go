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

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_InspectTFModuleConfig(t *testing.T) {
	tests := []struct {
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
				ContextVarExists:   false,
				RequiredProviders:  []string{"aws"},
				ResultOutputExists: false,
				Parameters:         map[string]any{},
			},
		},
		{
			name:       "aws provider with recipe context variable and output",
			workingDir: "testdata",
			moduleName: "test-module-recipe-context-outputs",
			result: &moduleInspectResult{
				ContextVarExists:   true,
				RequiredProviders:  []string{"aws"},
				ResultOutputExists: true,
				Parameters: map[string]any{
					"context": "contextInformation",
				},
			},
		},
		{
			name:       "invalid module name - non existent module directory",
			workingDir: "testdata",
			moduleName: "invalid-module",
			err:        "error loading the module",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := inspectModule(tc.workingDir, tc.moduleName)
			if tc.err != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.err)
				return
			}
			// Context parameters aren't treated as user-set parameters so we check for it's existence, but not its value
			if result.Parameters["context"] != nil {
				result.Parameters["context"] = "contextInformation"
			}
			require.NoError(t, err)
			require.Equal(t, tc.result, result)
		})
	}
}

func Test_DownloadModule_EmptyWorkingDirPath_Error(t *testing.T) {
	// Create a temporary test directory.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")

	err := downloadModule(testcontext.New(t), "", execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Terraform cannot be initialised with empty workdir")
}
