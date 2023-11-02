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

func TestNewTerraform_EmptyWorkingDirPath(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")

	// Call NewTerraform with an empty working directory path.
	_, err := NewTerraform(testcontext.New(t), "", execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Terraform cannot be initialised with empty workdir")
}

func TestNewTerraform_InvalidWorkingDirPath(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")

	// Call NewTerraform with an empty working directory path.
	_, err := NewTerraform(testcontext.New(t), "/invalid-dir", execPath)
	require.Error(t, err)
	require.Equal(t, err.Error(), "failed to initialize Terraform: error initialising Terraform with workdir /invalid-dir: stat /invalid-dir: no such file or directory")
}

func TestNewTerraform_EmptyExecPath(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	// Call NewTerraform with an empty working directory path.
	_, err := NewTerraform(testcontext.New(t), testDir, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize Terraform: no suitable terraform binary could be found")
}
