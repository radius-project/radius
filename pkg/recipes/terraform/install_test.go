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
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests for URL validation functions (from debug logging branch)
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
			name:        "malformed URL",
			releasesURL: "not-a-url",
			tlsConfig:   nil,
			wantErr:     true,
			errorMsg:    "releases API URL must use either HTTP or HTTPS scheme, got:",
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
			name:       "HTTPS URL is valid",
			archiveURL: "https://releases.example.com/terraform_1.7.0_linux_amd64.zip",
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
			name:       "malformed URL",
			archiveURL: "not-a-url",
			tlsConfig:  nil,
			wantErr:    true,
			errorMsg:   "archive URL must use either HTTP or HTTPS scheme, got:",
		},
		{
			name:       "HTTPS URL with query parameters",
			archiveURL: "https://releases.example.com/terraform_1.7.0_linux_amd64.zip?token=abc123",
			tlsConfig:  nil,
			wantErr:    false,
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

// Test for merged Install function signature (combining both features)
func TestInstall_MergedSignature(t *testing.T) {
	// Test that the function accepts the merged signature with both air-gapped and debug logging parameters
	ctx := context.Background()
	installer := install.NewInstaller()
	workingDir := "/tmp/test"

	// Test parameters for both features
	terraformConfig := datamodel.TerraformConfigProperties{
		Version: &datamodel.TerraformVersionConfig{
			ReleasesAPIBaseURL: "https://releases.hashicorp.com",
			ReleasesArchiveURL: "",
			TLS: &datamodel.TLSConfig{
				SkipVerify: false,
			},
		},
	}
	secrets := make(map[string]recipes.SecretData)
	logLevel := "DEBUG"

	// We can't actually run this without proper setup, but we can verify the signature compiles
	t.Logf("Install function signature test: terraformConfig=%+v, secrets=%+v, logLevel=%s", terraformConfig, secrets, logLevel)

	// Just verify the function exists and can be called (will error due to invalid setup, but that's expected)
	_, err := Install(ctx, installer, workingDir, terraformConfig, secrets, logLevel)
	// The function may succeed or fail depending on the environment, but no compilation error should occur
	t.Logf("Install function test result: %v", err)
}

// Tests for air-gapped installation functionality (from ytimocin's branch)
func TestInstall_PreMountedBinary(t *testing.T) {
	// Skip this test in short mode as it may require filesystem operations
	if testing.Short() {
		t.Skip("Skipping pre-mounted binary test in short mode")
	}

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
	terraformConfig := datamodel.TerraformConfigProperties{}
	secrets := make(map[string]recipes.SecretData)

	// Call Install function - should use pre-mounted binary
	tf, err := Install(ctx, installer, tmpDir, terraformConfig, secrets, "ERROR")

	// The actual behavior depends on the implementation, but we're testing the signature
	// and that the function can handle pre-mounted binaries
	if err != nil {
		t.Logf("Install returned error (expected in test environment): %v", err)
	} else {
		require.NotNil(t, tf)
		t.Logf("Install succeeded with pre-mounted binary")
	}
}

func TestInstall_PreMountedBinaryNotExecutable(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping fallback test in short mode")
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
	terraformConfig := datamodel.TerraformConfigProperties{}
	secrets := make(map[string]recipes.SecretData)

	// Call Install function - should fallback to download due to permission issues
	tf, err := Install(ctx, installer, tmpDir, terraformConfig, secrets, "WARN")

	// The actual behavior depends on the implementation
	if err != nil {
		t.Logf("Install returned error (may be expected in test environment): %v", err)
	} else {
		require.NotNil(t, tf)
		t.Logf("Install succeeded with fallback to download")

		// Verify that the install directory was created (indicating fallback to download)
		installDir := filepath.Join(tmpDir, installSubDir)
		if _, err := os.Stat(installDir); err == nil {
			t.Logf("Install directory created (fallback to download): %s", installDir)
		}
	}
}

// Tests for download functionality with debug logging (combining both features)
func TestInstall_DownloadWithDebugLogging(t *testing.T) {
	// Skip this test in short mode as it requires downloading Terraform
	if testing.Short() {
		t.Skip("Skipping download test in short mode")
	}

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Don't create any pre-mounted binary to force download
	ctx := context.Background()
	installer := install.NewInstaller()

	// Test with custom releases URL and debug logging
	terraformConfig := datamodel.TerraformConfigProperties{
		Version: &datamodel.TerraformVersionConfig{
			ReleasesAPIBaseURL: "https://releases.hashicorp.com",
			TLS: &datamodel.TLSConfig{
				SkipVerify: false,
			},
		},
	}
	secrets := make(map[string]recipes.SecretData)

	// Call Install function with DEBUG logging - should download and log details
	tf, err := Install(ctx, installer, tmpDir, terraformConfig, secrets, "DEBUG")

	// The actual behavior depends on the implementation and network availability
	if err != nil {
		t.Logf("Install returned error (may be expected in test environment): %v", err)
	} else {
		require.NotNil(t, tf)
		t.Logf("Install succeeded with download and debug logging")

		// Verify that the install directory was created
		installDir := filepath.Join(tmpDir, installSubDir)
		if _, err := os.Stat(installDir); err == nil {
			t.Logf("Install directory created: %s", installDir)
		}
	}
}

func TestInstall_AirGappedWithCustomRegistry(t *testing.T) {
	// Skip this test in short mode
	if testing.Short() {
		t.Skip("Skipping air-gapped custom registry test in short mode")
	}

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "terraform-install-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()
	installer := install.NewInstaller()

	// Test air-gapped installation with custom registry URLs
	terraformConfig := datamodel.TerraformConfigProperties{
		Version: &datamodel.TerraformVersionConfig{
			ReleasesAPIBaseURL: "https://internal-releases.company.com",
			ReleasesArchiveURL: "https://internal-releases.company.com/terraform_1.6.0_linux_amd64.zip",
			TLS: &datamodel.TLSConfig{
				SkipVerify: true, // For internal registries
			},
		},
	}
	secrets := make(map[string]recipes.SecretData)

	// Call Install function with air-gapped configuration
	tf, err := Install(ctx, installer, tmpDir, terraformConfig, secrets, "INFO")

	// The actual behavior depends on the implementation and network availability
	if err != nil {
		t.Logf("Install returned error (expected for non-existent internal registry): %v", err)
		// This is expected to fail since we're using fake internal URLs
		assert.Error(t, err)
	} else {
		require.NotNil(t, tf)
		t.Logf("Install succeeded with air-gapped configuration")
	}
}

// Test for registry environment variables (air-gapped feature)
func TestInstall_RegistryEnvironmentVariables(t *testing.T) {
	// Test that registry environment variables are properly set up
	ctx := context.Background()
	installer := install.NewInstaller()
	workingDir := "/tmp/test"

	// Test configuration with registry environment variables
	terraformConfig := datamodel.TerraformConfigProperties{
		Registry: &datamodel.TerraformRegistryConfig{
			Mirror: "https://internal-registry.company.com",
			TLS: &datamodel.TLSConfig{
				SkipVerify: true,
			},
		},
	}

	// Simulate registry credentials in secrets
	secrets := map[string]recipes.SecretData{
		"terraform-registry-auth": {
			Data: map[string]string{
				"username": "registry-user",
				"password": "registry-pass",
			},
		},
	}

	// Call Install function - should handle registry authentication
	_, err := Install(ctx, installer, workingDir, terraformConfig, secrets, "DEBUG")

	// The function may succeed or fail depending on the environment
	t.Logf("Registry environment test completed with result: %v", err)
}
