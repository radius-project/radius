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

	"github.com/project-radius/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestGetRequiredProviders(t *testing.T) {
	// Create a temporary test directory.
	testDir := t.TempDir()
	// Create a test module directory.
	moduleDir := filepath.Join(testDir, moduleRootDir, "test-module")
	err := os.MkdirAll(moduleDir, 0755)
	require.NoError(t, err)

	// Create a test provider file.
	providerFile := filepath.Join(moduleDir, "provider.tf")
	err = os.WriteFile(providerFile, []byte(`
	    terraform {
			required_providers {
				aws = {
					source = "hashicorp/aws"
					version = ">=3.0"
				}
			}
		}
    `), 0644)
	require.NoError(t, err)

	// Load the module to get required providers
	providers, err := getRequiredProviders(testDir, "test-module")
	require.NoError(t, err)

	// Assert that the loaded providers map contains the expected data.
	expectedProviders := []string{"aws"}
	require.Equal(t, expectedProviders, providers)
}

func TestGetRequiredProviders_Error(t *testing.T) {
	// Create a temporary test directory.
	testDir := t.TempDir()

	// Load the module with an invalid module name - non existent module directory
	_, err := getRequiredProviders(testDir, "invalid-module")

	// Assert that LoadModule returns an error.
	require.Error(t, err)
	require.Contains(t, err.Error(), "error loading the module")
}

func TestDownloadModule_EmptyWorkingDirPath_Error(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")

	err := downloadModule(testcontext.New(t), "", execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Terraform cannot be initialised with empty workdir")
}
