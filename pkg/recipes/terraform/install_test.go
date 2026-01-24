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
	tf, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir, LogLevel: "DEBUG"})
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

	tf1, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir1, LogLevel: "ERROR"})
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

	tf2, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir2, LogLevel: "DEBUG"})
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
	tf1, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir1, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf1)

	tf2, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir2, LogLevel: "DEBUG"})
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

func TestInstall_InstallerAPIBinaryPriority(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for the installer API location
	installerTmpDir, err := os.MkdirTemp("", "terraform-installer-api-test")
	require.NoError(t, err)
	defer os.RemoveAll(installerTmpDir)
	// Resolve symlinks for consistent path comparison (macOS /var -> /private/var)
	installerTmpDir, err = filepath.EvalSymlinks(installerTmpDir)
	require.NoError(t, err)

	// Create a temporary directory for the global terraform location (fallback)
	globalTmpDir, err := os.MkdirTemp("", "terraform-global-fallback-test")
	require.NoError(t, err)
	defer os.RemoveAll(globalTmpDir)

	// Set environment variables to override paths for testing
	oldGlobalEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalTmpDir)
	defer os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldGlobalEnv)

	oldInstallerEnv := os.Getenv("TERRAFORM_TEST_INSTALLER_DIR")
	os.Setenv("TERRAFORM_TEST_INSTALLER_DIR", installerTmpDir)
	defer os.Setenv("TERRAFORM_TEST_INSTALLER_DIR", oldInstallerEnv)

	// Reset global state for this test
	resetGlobalStateForTesting()

	ctx := context.Background()
	installer := install.NewInstaller()

	// First, install terraform to a "versions" subdirectory (simulating installer API)
	versionsDir := filepath.Join(installerTmpDir, "versions", "1.6.4")
	require.NoError(t, os.MkdirAll(versionsDir, 0755))

	// Download terraform to the versions directory
	tmpDir, err := os.MkdirTemp("", "terraform-download-helper")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Use Install to download terraform first (to a temp location)
	tf, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir, LogLevel: "ERROR"})
	require.NoError(t, err)

	// Copy the downloaded binary to our simulated installer location
	execPath := tf.ExecPath()
	binaryData, err := os.ReadFile(execPath)
	require.NoError(t, err)

	installerBinaryPath := filepath.Join(versionsDir, "terraform")
	require.NoError(t, os.WriteFile(installerBinaryPath, binaryData, 0755))

	// Create the "current" symlink pointing to the version binary
	currentSymlink := filepath.Join(installerTmpDir, "current")
	require.NoError(t, os.Symlink(installerBinaryPath, currentSymlink))

	// Clean up the global directory that was populated during the helper download.
	// This ensures we can test that the installer binary takes priority and no
	// new global binary is created.
	require.NoError(t, os.RemoveAll(globalTmpDir))
	require.NoError(t, os.MkdirAll(globalTmpDir, 0755))

	// Reset state again to test fresh lookup
	resetGlobalStateForTesting()

	// Now Install should use the installer API binary via the symlink
	tmpDir2, err := os.MkdirTemp("", "terraform-execution-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir2)

	tf2, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir2, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf2)

	// Verify it's using the installer binary path
	require.Equal(t, installerBinaryPath, tf2.ExecPath(), "Should use installer API binary path")

	// Verify no global binary was created (we used installer binary)
	globalBinary := filepath.Join(globalTmpDir, "terraform")
	_, err = os.Stat(globalBinary)
	require.True(t, os.IsNotExist(err), "Global binary should not be created when installer binary exists")
}

func TestInstall_InstallerSymlinkChangeInvalidatesCache(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// This test verifies that when the installer symlink is updated to point to a
	// different version, the cached binary path is invalidated and the new version
	// is used. This is critical for `rad terraform install --version X` to take
	// effect without requiring a pod restart.

	// Create a temporary directory for the installer API location
	installerTmpDir, err := os.MkdirTemp("", "terraform-symlink-change-test")
	require.NoError(t, err)
	defer os.RemoveAll(installerTmpDir)
	// Resolve symlinks for consistent path comparison (macOS /var -> /private/var)
	installerTmpDir, err = filepath.EvalSymlinks(installerTmpDir)
	require.NoError(t, err)

	// Create a temporary directory for the global terraform location (fallback)
	globalTmpDir, err := os.MkdirTemp("", "terraform-global-symlink-test")
	require.NoError(t, err)
	defer os.RemoveAll(globalTmpDir)

	// Set environment variables to override paths for testing
	oldGlobalEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", globalTmpDir)
	defer os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldGlobalEnv)

	oldInstallerEnv := os.Getenv("TERRAFORM_TEST_INSTALLER_DIR")
	os.Setenv("TERRAFORM_TEST_INSTALLER_DIR", installerTmpDir)
	defer os.Setenv("TERRAFORM_TEST_INSTALLER_DIR", oldInstallerEnv)

	// Reset global state for this test
	resetGlobalStateForTesting()

	ctx := context.Background()
	installer := install.NewInstaller()

	// Download terraform binary to use for testing
	tmpDir, err := os.MkdirTemp("", "terraform-download-helper")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Temporarily unset installer dir so we download to global
	os.Unsetenv("TERRAFORM_TEST_INSTALLER_DIR")
	tf, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir, LogLevel: "ERROR"})
	require.NoError(t, err)
	os.Setenv("TERRAFORM_TEST_INSTALLER_DIR", installerTmpDir)

	// Copy the downloaded binary to two simulated versions
	execPath := tf.ExecPath()
	binaryData, err := os.ReadFile(execPath)
	require.NoError(t, err)

	// Create version 1.6.4
	version164Dir := filepath.Join(installerTmpDir, "versions", "1.6.4")
	require.NoError(t, os.MkdirAll(version164Dir, 0755))
	binary164Path := filepath.Join(version164Dir, "terraform")
	require.NoError(t, os.WriteFile(binary164Path, binaryData, 0755))

	// Create version 1.7.0
	version170Dir := filepath.Join(installerTmpDir, "versions", "1.7.0")
	require.NoError(t, os.MkdirAll(version170Dir, 0755))
	binary170Path := filepath.Join(version170Dir, "terraform")
	require.NoError(t, os.WriteFile(binary170Path, binaryData, 0755))

	// Create the "current" symlink pointing to version 1.6.4
	currentSymlink := filepath.Join(installerTmpDir, "current")
	require.NoError(t, os.Symlink(binary164Path, currentSymlink))

	// Reset state to test fresh lookup with installer symlink
	resetGlobalStateForTesting()

	// First Install call - should use version 1.6.4 via symlink
	tmpDir1, err := os.MkdirTemp("", "terraform-execution-1")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir1)

	tf1, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir1, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf1)
	require.Equal(t, binary164Path, tf1.ExecPath(), "First call should use 1.6.4 binary")

	// Simulate `rad terraform install --version 1.7.0` by updating the symlink
	require.NoError(t, os.Remove(currentSymlink))
	require.NoError(t, os.Symlink(binary170Path, currentSymlink))

	// Second Install call - should detect symlink change and use version 1.7.0
	// NOTE: Without the fix, this would incorrectly return the cached 1.6.4 path
	tmpDir2, err := os.MkdirTemp("", "terraform-execution-2")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir2)

	tf2, err := Install(ctx, installer, InstallOptions{RootDir: tmpDir2, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf2)
	require.Equal(t, binary170Path, tf2.ExecPath(), "Second call should use 1.7.0 binary after symlink change")

	// Verify both terraform instances work
	_, _, err = tf1.Version(ctx, false)
	require.NoError(t, err, "First terraform instance should work")

	_, _, err = tf2.Version(ctx, false)
	require.NoError(t, err, "Second terraform instance should work")
}

func TestInstall_TerraformPathChangeInvalidatesCache(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// This test verifies that when TerraformPath changes between calls,
	// the cache is invalidated and the new root's binary is used.
	// This prevents returning a binary from the wrong root in multi-tenant
	// scenarios or when configuration changes.

	// Create two separate terraform root directories
	root1, err := os.MkdirTemp("", "terraform-root1")
	require.NoError(t, err)
	defer os.RemoveAll(root1)
	// Resolve symlinks for consistent path comparison (macOS /var -> /private/var)
	root1, err = filepath.EvalSymlinks(root1)
	require.NoError(t, err)

	root2, err := os.MkdirTemp("", "terraform-root2")
	require.NoError(t, err)
	defer os.RemoveAll(root2)
	// Resolve symlinks for consistent path comparison (macOS /var -> /private/var)
	root2, err = filepath.EvalSymlinks(root2)
	require.NoError(t, err)

	// Reset global state for this test
	resetGlobalStateForTesting()

	ctx := context.Background()
	installer := install.NewInstaller()

	// Download terraform binary to use for testing
	helperDir, err := os.MkdirTemp("", "terraform-download-helper")
	require.NoError(t, err)
	defer os.RemoveAll(helperDir)

	// Clear env vars to use TerraformPath directly
	oldGlobalEnv := os.Getenv("TERRAFORM_TEST_GLOBAL_DIR")
	oldInstallerEnv := os.Getenv("TERRAFORM_TEST_INSTALLER_DIR")
	os.Unsetenv("TERRAFORM_TEST_GLOBAL_DIR")
	os.Unsetenv("TERRAFORM_TEST_INSTALLER_DIR")
	defer func() {
		os.Setenv("TERRAFORM_TEST_GLOBAL_DIR", oldGlobalEnv)
		os.Setenv("TERRAFORM_TEST_INSTALLER_DIR", oldInstallerEnv)
	}()

	// Download terraform to helper directory first
	helperTf, err := Install(ctx, installer, InstallOptions{RootDir: helperDir, TerraformPath: helperDir, LogLevel: "ERROR"})
	require.NoError(t, err)
	binaryData, err := os.ReadFile(helperTf.ExecPath())
	require.NoError(t, err)

	// Set up root1 with installer symlink
	root1VersionDir := filepath.Join(root1, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(root1VersionDir, 0755))
	root1Binary := filepath.Join(root1VersionDir, "terraform")
	require.NoError(t, os.WriteFile(root1Binary, binaryData, 0755))
	root1Symlink := filepath.Join(root1, "current")
	require.NoError(t, os.Symlink(root1Binary, root1Symlink))

	// Set up root2 with installer symlink
	root2VersionDir := filepath.Join(root2, "versions", "2.0.0")
	require.NoError(t, os.MkdirAll(root2VersionDir, 0755))
	root2Binary := filepath.Join(root2VersionDir, "terraform")
	require.NoError(t, os.WriteFile(root2Binary, binaryData, 0755))
	root2Symlink := filepath.Join(root2, "current")
	require.NoError(t, os.Symlink(root2Binary, root2Symlink))

	// Reset state to test fresh lookup
	resetGlobalStateForTesting()

	// First Install call with TerraformPath = root1
	execDir1, err := os.MkdirTemp("", "terraform-exec-1")
	require.NoError(t, err)
	defer os.RemoveAll(execDir1)

	tf1, err := Install(ctx, installer, InstallOptions{RootDir: execDir1, TerraformPath: root1, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf1)
	require.Equal(t, root1Binary, tf1.ExecPath(), "First call should use root1 binary")

	// Second Install call with TerraformPath = root2 (different root)
	// This should invalidate the cache and use root2's binary
	execDir2, err := os.MkdirTemp("", "terraform-exec-2")
	require.NoError(t, err)
	defer os.RemoveAll(execDir2)

	tf2, err := Install(ctx, installer, InstallOptions{RootDir: execDir2, TerraformPath: root2, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf2)
	require.Equal(t, root2Binary, tf2.ExecPath(), "Second call should use root2 binary after TerraformPath change")

	// Third Install call back to root1 - should switch back
	execDir3, err := os.MkdirTemp("", "terraform-exec-3")
	require.NoError(t, err)
	defer os.RemoveAll(execDir3)

	tf3, err := Install(ctx, installer, InstallOptions{RootDir: execDir3, TerraformPath: root1, LogLevel: "ERROR"})
	require.NoError(t, err)
	require.NotNil(t, tf3)
	require.Equal(t, root1Binary, tf3.ExecPath(), "Third call should switch back to root1 binary")

	// Verify all terraform instances work
	_, _, err = tf1.Version(ctx, false)
	require.NoError(t, err, "First terraform instance should work")
	_, _, err = tf2.Version(ctx, false)
	require.NoError(t, err, "Second terraform instance should work")
	_, _, err = tf3.Version(ctx, false)
	require.NoError(t, err, "Third terraform instance should work")
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
			tf, err := Install(ctx, installer, InstallOptions{RootDir: dir, LogLevel: "ERROR"})
			if err != nil {
				errors <- err
				return
			}
			results <- tf
		}(tmpDir)
	}

	var terraforms []*tfexec.Terraform
	for range len(tmpDirs) {
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
