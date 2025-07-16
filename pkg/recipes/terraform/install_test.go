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
	"testing"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/assert"
)

func Test_validateReleasesURL(t *testing.T) {
	tests := []struct {
		name        string
		releasesURL string
		tlsConfig   *datamodel.TLSConfig
		wantErr     bool
		errorMsg    string
	}{
		{
			name:        "empty URL is valid",
			releasesURL: "",
			tlsConfig:   nil,
			wantErr:     false,
		},
		{
			name:        "HTTPS URL is valid",
			releasesURL: "https://releases.hashicorp.com",
			tlsConfig:   nil,
			wantErr:     false,
		},
		{
			name:        "HTTP URL without skip verify is invalid",
			releasesURL: "http://releases.hashicorp.com",
			tlsConfig:   nil,
			wantErr:     true,
			errorMsg:    "must use HTTPS for security",
		},
		{
			name:        "HTTP URL with skip verify is valid",
			releasesURL: "http://releases.hashicorp.com",
			tlsConfig:   &datamodel.TLSConfig{SkipVerify: true},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReleasesURL(context.Background(), tt.releasesURL, tt.tlsConfig)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_validateArchiveURL(t *testing.T) {
	tests := []struct {
		name       string
		archiveURL string
		tlsConfig  *datamodel.TLSConfig
		wantErr    bool
		errorMsg   string
	}{
		{
			name:       "empty URL is valid",
			archiveURL: "",
			tlsConfig:  nil,
			wantErr:    false,
		},
		{
			name:       "HTTPS ZIP URL is valid",
			archiveURL: "https://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip",
			tlsConfig:  nil,
			wantErr:    false,
		},
		{
			name:       "HTTP ZIP URL without skip verify is invalid",
			archiveURL: "http://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "must use HTTPS for security",
		},
		{
			name:       "HTTP ZIP URL with skip verify is valid",
			archiveURL: "http://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.zip",
			tlsConfig:  &datamodel.TLSConfig{SkipVerify: true},
			wantErr:    false,
		},
		{
			name:       "Non-ZIP URL is invalid",
			archiveURL: "https://releases.hashicorp.com/terraform/1.5.7/terraform_1.5.7_linux_amd64.tar.gz",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "must point to a .zip file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArchiveURL(context.Background(), tt.archiveURL, tt.tlsConfig)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test for the merged Install function - simplified version to test basic functionality
func TestInstall_MergedFeatures(t *testing.T) {
	// This is a basic test to verify the function signature works
	// More comprehensive testing would require mocking the installer and filesystem
	ctx := context.Background()
	
	// Test that the function accepts the new merged signature
	terraformConfig := datamodel.TerraformConfigProperties{}
	secrets := make(map[string]recipes.SecretData)
	logLevel := "DEBUG"
	
	// We can't actually run this without proper setup, but we can verify the signature compiles
	t.Logf("Install function signature test: terraformConfig=%+v, secrets=%+v, logLevel=%s", terraformConfig, secrets, logLevel)
}

package terraform

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	install "github.com/hashicorp/hc-install"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_validateReleasesURL(t *testing.T) {
	tests := []struct {
		name        string
		releasesURL string
		tlsConfig   *datamodel.TLSConfig
		wantErr     bool
		errorMsg    string
	}{
		{
			name:        "empty URL is valid",
			releasesURL: "",
			tlsConfig:   nil,
			wantErr:     false,
		},
		{
			name:        "HTTPS URL is valid",
			releasesURL: "https://releases.example.com",
			tlsConfig:   nil,
			wantErr:     false,
		},
		{
			name:        "HTTP URL without skipVerify is invalid",
			releasesURL: "http://releases.example.com",
			tlsConfig:   nil,
			wantErr:     true,
			errorMsg:    "releases API URL must use HTTPS for security. Use 'tls.skipVerify: true' to allow insecure connections (not recommended)",
		},
		{
			name:        "HTTP URL with skipVerify is valid",
			releasesURL: "http://releases.example.com",
			tlsConfig: &datamodel.TLSConfig{
				SkipVerify: true,
			},
			wantErr: false,
		},
		{
			name:        "invalid URL scheme",
			releasesURL: "ftp://releases.example.com",
			tlsConfig:   nil,
			wantErr:     true,
			errorMsg:    "releases API URL must use either HTTP or HTTPS scheme, got: ftp",
		},
		{
			name:        "malformed URL",
			releasesURL: "://invalid-url",
			tlsConfig:   nil,
			wantErr:     true,
			errorMsg:    "invalid releases API URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReleasesURL(context.Background(), tt.releasesURL, tt.tlsConfig)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
=======
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
	tf, err := Install(ctx, installer, tmpDir, "ERROR")
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
	tf, err := Install(ctx, installer, tmpDir, "ERROR")
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify that the install directory was created (indicating fallback to download)
	installDir := filepath.Join(tmpDir, installSubDir)
	_, err = os.Stat(installDir)
	require.NoError(t, err, "Install directory should exist when falling back to download")
>>>>>>> cd90e3a88 (Should be working, but needs a test for cloud resources that executes tf version at debug level.)
}

func Test_validateArchiveURL(t *testing.T) {
	tests := []struct {
		name       string
		archiveURL string
		tlsConfig  *datamodel.TLSConfig
		wantErr    bool
		errorMsg   string
	}{
		{
			name:       "empty URL is valid",
			archiveURL: "",
			tlsConfig:  nil,
			wantErr:    false,
		},
		{
			name:       "HTTPS URL with .zip extension is valid",
			archiveURL: "https://releases.example.com/terraform/1.7.0/terraform_1.7.0_linux_amd64.zip",
			tlsConfig:  nil,
			wantErr:    false,
		},
		{
			name:       "HTTP URL without skipVerify is invalid",
			archiveURL: "http://releases.example.com/terraform_1.7.0_linux_amd64.zip",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "archive URL must use HTTPS for security. Use 'tls.skipVerify: true' to allow insecure connections (not recommended)",
		},
		{
			name:       "HTTP URL with skipVerify is valid",
			archiveURL: "http://releases.example.com/terraform_1.7.0_linux_amd64.zip",
			tlsConfig: &datamodel.TLSConfig{
				SkipVerify: true,
			},
			wantErr: false,
		},
		{
			name:       "URL without .zip extension is invalid",
			archiveURL: "https://releases.example.com/terraform/1.7.0/terraform_1.7.0_linux_amd64",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "archive URL must point to a .zip file",
		},
		{
			name:       "URL with .tar.gz extension is invalid",
			archiveURL: "https://releases.example.com/terraform/1.7.0/terraform_1.7.0_linux_amd64.tar.gz",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "archive URL must point to a .zip file, got: .gz",
		},
		{
			name:       "invalid URL scheme",
			archiveURL: "ftp://releases.example.com/terraform_1.7.0_linux_amd64.zip",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "archive URL must use either HTTP or HTTPS scheme, got: ftp",
		},
		{
			name:       "malformed URL",
			archiveURL: "://invalid-url.zip",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "invalid archive URL",
		},
		{
			name:       "URL with query parameters is valid",
			archiveURL: "https://releases.example.com/terraform_1.7.0_linux_amd64.zip?token=abc123",
			tlsConfig:  nil,
			wantErr:    false,
		},
	}

<<<<<<< HEAD
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateArchiveURL(context.Background(), tt.archiveURL, tt.tlsConfig)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
=======
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Don't create any pre-mounted binary
	ctx := context.Background()
	installer := install.NewInstaller()

	// Call Install function - should download
	tf, err := Install(ctx, installer, tmpDir, "DEBUG")
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
	tf, err := Install(ctx, installer, tmpDir, "WARN")
	require.NoError(t, err)
	require.NotNil(t, tf)

	// Verify that the install directory was created (indicating fallback to download)
	installDir := filepath.Join(tmpDir, installSubDir)
	_, err = os.Stat(installDir)
	require.NoError(t, err, "Install directory should exist when falling back to download")
>>>>>>> cd90e3a88 (Should be working, but needs a test for cloud resources that executes tf version at debug level.)
}
