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

	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func TestNewTerraform_Success(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	execPath := filepath.Join(testDir, "terraform")
	expectedWorkingDir := filepath.Join(testDir, executionSubDir)

	tf, err := NewTerraform(testcontext.New(t), testDir, execPath)
	require.NoError(t, err)
	require.Equal(t, expectedWorkingDir, tf.WorkingDir())
}

func TestNewTerraform_InvalidDir(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()
	// Create a read-only directory within the temporary directory.
	readOnlyDir := filepath.Join(testDir, "read-only-dir")
	err := os.MkdirAll(readOnlyDir, 0555)
	require.NoError(t, err)

	execPath := filepath.Join(testDir, "terraform")

	// Call NewTerraform with read only root directory.
	_, err = NewTerraform(testcontext.New(t), readOnlyDir, execPath)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create working directory for terraform execution")
}

func TestNewTerraform_EmptyExecPath(t *testing.T) {
	// Create a temporary directory for testing.
	testDir := t.TempDir()

	// Call NewTerraform with an empty exec path.
	_, err := NewTerraform(testcontext.New(t), testDir, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to initialize Terraform: no suitable terraform binary could be found")
}

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
