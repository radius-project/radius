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
	"os"
	"path/filepath"
	"testing"

	install "github.com/hashicorp/hc-install"
	"github.com/stretchr/testify/require"
)

func TestInstall_PreMountedBinary(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a fake Terraform binary in the expected pre-mounted location
	preMountedPath := filepath.Join(tmpDir, "terraform")

	// Create a simple script that acts like terraform and responds to version command
	terraformScript := `#!/bin/bash
if [ "$1" = "version" ]; then
    echo "Terraform v1.6.0"
    echo "on linux_amd64"
    exit 0
fi
exit 1
`
	err = os.WriteFile(preMountedPath, []byte(terraformScript), 0755)
	require.NoError(t, err)

	ctx := context.Background()
	installer := install.NewInstaller()

	// Call Install function
	tf, err := Install(ctx, installer, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify that the pre-mounted binary was used
	// We can't easily verify the exact path without exposing internals,
	// but we can verify that no download directory was created
	installDir := filepath.Join(tmpDir, installSubDir)
	_, err = os.Stat(installDir)
	require.True(t, os.IsNotExist(err), "Install directory should not exist when using pre-mounted binary")
}

func TestInstall_PreMountedBinaryInvalid_FallbackToDownload(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create an invalid/broken "terraform" binary
	preMountedPath := filepath.Join(tmpDir, "terraform")
	err = os.WriteFile(preMountedPath, []byte("invalid binary"), 0755)
	require.NoError(t, err)

	ctx := context.Background()
	installer := install.NewInstaller()

	// Call Install function - should fallback to download
	tf, err := Install(ctx, installer, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify that the install directory was created (indicating fallback to download)
	installDir := filepath.Join(tmpDir, installSubDir)
	_, err = os.Stat(installDir)
	require.NoError(t, err, "Install directory should exist when falling back to download")
}

func TestInstall_NoPreMountedBinary_Download(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Don't create any pre-mounted binary
	ctx := context.Background()
	installer := install.NewInstaller()

	// Call Install function - should download
	tf, err := Install(ctx, installer, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify that the install directory was created
	installDir := filepath.Join(tmpDir, installSubDir)
	_, err = os.Stat(installDir)
	require.NoError(t, err, "Install directory should exist when downloading")
}

func TestInstall_PreMountedBinaryNotExecutable(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a file that's not executable
	preMountedPath := filepath.Join(tmpDir, "terraform")
	err = os.WriteFile(preMountedPath, []byte("#!/bin/bash\necho 'test'"), 0644) // No execute permission
	require.NoError(t, err)

	ctx := context.Background()
	installer := install.NewInstaller()

	// Call Install function - should fallback to download due to permission issues
	tf, err := Install(ctx, installer, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify that the install directory was created (indicating fallback to download)
	installDir := filepath.Join(tmpDir, installSubDir)
	_, err = os.Stat(installDir)
	require.NoError(t, err, "Install directory should exist when falling back to download")
}
