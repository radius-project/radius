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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestInstall_SuccessfulDownload(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for the global terraform location
	globalTmpDir, err := os.MkdirTemp("", "terraform-download-test")
	require.NoError(t, err)
	defer os.RemoveAll(globalTmpDir)

	// Set environment variable to override global terraform path for testing
	oldEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalTmpDir)
	defer os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldEnv)

	// Reset global state for this test
	resetGlobalStateForTesting()

	// Create a temporary execution directory (simulating normal usage)
	tmpDir, err := os.MkdirTemp("", "terraform-execution-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	installer := install.NewInstaller()

	// Call Install function - should download to global location
	tf, err := Install(ctx, installer, tmpDir, datamodel.TerraformConfigProperties{}, nil)
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify terraform binary works
	_, _, err = tf.Version(ctx, false)
	require.NoError(t, err, "Terraform version check should work")

	// Verify global terraform directory was created with expected files
	globalBinary := filepath.Join(globalTmpDir, "terraform")
	globalMarker := filepath.Join(globalTmpDir, ".terraform-ready")

	_, err = os.Stat(globalBinary)
	require.NoError(t, err, "Global terraform binary should exist")

	_, err = os.Stat(globalMarker)
	require.NoError(t, err, "Global terraform marker file should exist")
}

func TestInstall_GlobalBinaryReuse(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for the global terraform location
	globalTmpDir, err := os.MkdirTemp("", "terraform-reuse-test")
	require.NoError(t, err)
	defer os.RemoveAll(globalTmpDir)

	// Set environment variable to override global terraform path for testing
	oldEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalTmpDir)
	defer os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldEnv)

	// Reset global state for this test
	resetGlobalStateForTesting()

	ctx := context.Background()
	installer := install.NewInstaller()

	// First Install call - should download
	tmpDir1, err := os.MkdirTemp("", "terraform-execution-test-1")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir1)

	tf1, err := Install(ctx, installer, tmpDir1, datamodel.TerraformConfigProperties{}, nil)
	require.NoError(t, err)
	require.NotNil(t, tf1)

	// Verify global files exist after first install
	globalBinary := filepath.Join(globalTmpDir, "terraform")
	globalMarker := filepath.Join(globalTmpDir, ".terraform-ready")

	_, err = os.Stat(globalBinary)
	require.NoError(t, err, "Global terraform binary should exist after first install")

	_, err = os.Stat(globalMarker)
	require.NoError(t, err, "Global terraform marker should exist after first install")

	// Second Install call - should reuse existing binary (no download)
	tmpDir2, err := os.MkdirTemp("", "terraform-execution-test-2")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir2)

	tf2, err := Install(ctx, installer, tmpDir2, datamodel.TerraformConfigProperties{}, nil)
	require.NoError(t, err)
	require.NotNil(t, tf2)

	// Both should work
	_, _, err = tf1.Version(ctx, false)
	require.NoError(t, err, "First terraform instance should work")

	_, _, err = tf2.Version(ctx, false)
	require.NoError(t, err, "Second terraform instance should work")
}

func TestInstall_MultipleConcurrentCallsUseSameBinary(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for the global terraform location
	globalTmpDir, err := os.MkdirTemp("", "terraform-same-binary-test")
	require.NoError(t, err)
	defer os.RemoveAll(globalTmpDir)

	// Set environment variable to override global terraform path for testing
	oldEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalTmpDir)
	defer os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldEnv)

	// Reset global state for this test
	resetGlobalStateForTesting()

	ctx := context.Background()
	installer := install.NewInstaller()

	// Create temporary execution directories
	tmpDir1, err := os.MkdirTemp("", "terraform-execution-1")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir1)

	tmpDir2, err := os.MkdirTemp("", "terraform-execution-2")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir2)

	// Call Install from different execution contexts
	tf1, err := Install(ctx, installer, tmpDir1, datamodel.TerraformConfigProperties{}, nil)
	require.NoError(t, err)
	require.NotNil(t, tf1)

	tf2, err := Install(ctx, installer, tmpDir2, datamodel.TerraformConfigProperties{}, nil)
	require.NoError(t, err)
	require.NotNil(t, tf2)

	// Both should work and be using terraform from global location
	_, _, err = tf1.Version(ctx, false)
	require.NoError(t, err, "First terraform instance should work")

	_, _, err = tf2.Version(ctx, false)
	require.NoError(t, err, "Second terraform instance should work")

	// Verify only one set of global files exists
	globalBinary := filepath.Join(globalTmpDir, "terraform")
	globalMarker := filepath.Join(globalTmpDir, ".terraform-ready")

	_, err = os.Stat(globalBinary)
	require.NoError(t, err, "Global terraform binary should exist")

	_, err = os.Stat(globalMarker)
	require.NoError(t, err, "Global terraform marker should exist")

	// No per-execution install directories should be created
	installDir1 := filepath.Join(tmpDir1, installSubDir)
	_, err = os.Stat(installDir1)
	require.True(t, os.IsNotExist(err), "No per-execution install directory should exist in first tmpDir")

	installDir2 := filepath.Join(tmpDir2, installSubDir)
	_, err = os.Stat(installDir2)
	require.True(t, os.IsNotExist(err), "No per-execution install directory should exist in second tmpDir")
}

func TestInstall_GlobalBinaryConcurrency(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for the global terraform location
	globalTmpDir, err := os.MkdirTemp("", "terraform-global-test")
	require.NoError(t, err)
	defer os.RemoveAll(globalTmpDir)

	// Set environment variable to override global terraform path for testing
	oldEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalTmpDir)
	defer os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldEnv)

	// Reset global state for this test
	resetGlobalStateForTesting()

	// Test that multiple concurrent Install calls use the same binary without race conditions
	ctx := context.Background()
	installer := install.NewInstaller()

	// Create multiple execution directories (as would happen in production)
	tmpDirs := make([]string, 3)
	for i := range tmpDirs {
		tmpDir, err := os.MkdirTemp("", fmt.Sprintf("terraform-concurrent-test-%d", i))
		require.NoError(t, err)
		defer os.RemoveAll(tmpDir)
		tmpDirs[i] = tmpDir
	}

	results := make(chan *tfexec.Terraform, len(tmpDirs))
	errors := make(chan error, len(tmpDirs))

	for _, tmpDir := range tmpDirs {
		go func(dir string) {
			tf, err := Install(ctx, installer, dir, datamodel.TerraformConfigProperties{}, nil)
			if err != nil {
				errors <- err
				return
			}
			results <- tf
		}(tmpDir)
	}

	var terraforms []*tfexec.Terraform
	for i := 0; i < len(tmpDirs); i++ {
		select {
		case tf := <-results:
			terraforms = append(terraforms, tf)
		case err := <-errors:
			require.NoError(t, err, "Concurrent Install call failed")
		}
	}

	require.Len(t, terraforms, len(tmpDirs), "All Install calls should succeed")
	for i, tf := range terraforms {
		require.NotNil(t, tf, "Terraform instance %d should not be nil", i)
		// Verify terraform binary works
		_, _, err := tf.Version(ctx, false)
		require.NoError(t, err, "Terraform version check should work for instance %d", i)
	}
}
