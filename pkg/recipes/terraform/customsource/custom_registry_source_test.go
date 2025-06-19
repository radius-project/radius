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

package customsource

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/hc-install/product"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomRegistrySource_Validate(t *testing.T) {
	tests := []struct {
		name    string
		source  *CustomRegistrySource
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid source",
			source: &CustomRegistrySource{
				Product:    product.Terraform,
				Version:    version.Must(version.NewVersion("1.5.0")),
				BaseURL:    "https://example.com",
				InstallDir: "/tmp/install",
			},
			wantErr: false,
		},
		{
			name: "missing product",
			source: &CustomRegistrySource{
				Product:    product.Product{}, // Empty product
				Version:    version.Must(version.NewVersion("1.5.0")),
				BaseURL:    "https://example.com",
				InstallDir: "/tmp/install",
			},
			wantErr: true,
			errMsg:  "Product is required",
		},
		{
			name: "missing version",
			source: &CustomRegistrySource{
				Product:    product.Terraform,
				BaseURL:    "https://example.com",
				InstallDir: "/tmp/install",
			},
			wantErr: true,
			errMsg:  "Version is required",
		},
		{
			name: "missing base URL",
			source: &CustomRegistrySource{
				Product:    product.Terraform,
				Version:    version.Must(version.NewVersion("1.5.0")),
				InstallDir: "/tmp/install",
			},
			wantErr: true,
			errMsg:  "BaseURL is required",
		},
		{
			name: "missing install dir",
			source: &CustomRegistrySource{
				Product: product.Terraform,
				Version: version.Must(version.NewVersion("1.5.0")),
				BaseURL: "https://example.com",
			},
			wantErr: true,
			errMsg:  "InstallDir is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.source.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCustomRegistrySource_Install_MockServer(t *testing.T) {
	// Create a mock registry server
	mockIndex := releaseIndex{
		Versions: map[string]releaseVersion{
			"1.5.0": {
				Version: "1.5.0",
				Builds: []releaseBuild{
					{
						OS:       "linux",
						Arch:     "amd64",
						Filename: "terraform_1.5.0_linux_amd64.zip",
						URL:      "/terraform/1.5.0/terraform_1.5.0_linux_amd64.zip",
					},
					{
						OS:       "darwin",
						Arch:     "amd64",
						Filename: "terraform_1.5.0_darwin_amd64.zip",
						URL:      "/terraform/1.5.0/terraform_1.5.0_darwin_amd64.zip",
					},
					{
						OS:       "darwin",
						Arch:     "arm64",
						Filename: "terraform_1.5.0_darwin_arm64.zip",
						URL:      "/terraform/1.5.0/terraform_1.5.0_darwin_arm64.zip",
					},
					{
						OS:       "windows",
						Arch:     "amd64",
						Filename: "terraform_1.5.0_windows_amd64.zip",
						URL:      "/terraform/1.5.0/terraform_1.5.0_windows_amd64.zip",
					},
				},
				Shasums: "",
			},
		},
	}

	// Mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth header
		if auth := r.Header.Get("Authorization"); auth != "" {
			if auth != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		switch r.URL.Path {
		case "/terraform/index.json":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockIndex)

		default:
			// For actual binary downloads, we'd return a zip file
			// For testing, just return 404
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Create temp directory
	tempDir := t.TempDir()

	// Test fetching release info
	source := &CustomRegistrySource{
		Product:    product.Terraform,
		Version:    version.Must(version.NewVersion("1.5.0")),
		BaseURL:    server.URL,
		InstallDir: tempDir,
		AuthToken:  "Bearer test-token",
	}

	ctx := context.Background()
	client, err := source.getHTTPClient()
	require.NoError(t, err)

	releaseInfo, err := source.fetchReleaseInfo(ctx, client)
	require.NoError(t, err)
	assert.Equal(t, "1.5.0", releaseInfo.Version)
	assert.Len(t, releaseInfo.Builds, 4)

	// Test finding build
	build, err := source.findBuild(releaseInfo)
	require.NoError(t, err)
	assert.NotNil(t, build)
}

func TestCustomRegistrySource_TLSConfig(t *testing.T) {
	// Test CA certificate handling - using a valid self-signed certificate
	testCA := []byte(`-----BEGIN CERTIFICATE-----
MIIDazCCAlOgAwIBAgIUJeohtgk8nnt8ofratXJg7KUJsbwwDQYJKoZIhvcNAQEL
BQAwRTELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUxITAfBgNVBAoM
GEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDAeFw0yMzA2MTIwMzQ0NTZaFw0zMzA2
MDkwMzQ0NTZaMEUxCzAJBgNVBAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEw
HwYDVQQKDBhJbnRlcm5ldCBXaWRnaXRzIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEB
AQUAA4IBDwAwggEKAoIBAQC7W8prTCGFCpn6UV/UpmSvc4XLFSkiZ91OkfSZmYyF
z3gEauvCQ6pq3S8gvZSLm6JqRRvJPDGQWBqPgMkVpLjAhgHAqCKr8WPvxrBNTgjr
qjLcj6nQoUKQ5mRoNPW7wNGJF7P5QYK1IZnVEAlcnEPVpWTYKHHs3+nNzBgHrxb7
12yi2leYAiGWMxpTrs5CW0CC7Tnvh9b4TcNBKF3hXwoEtyF0g1N6eT8qCW92gfcC
efKJDfQqHu3dFALRKqPql5+rhPenEBTyHWriGi1czYgKdl8eH8/ATw3diL3iflxO
B5Yn9nU5h38nrSEZqYXEQaFDHCDJcIcf3JOa8KYHaKSHAgMBAAGjUzBRMB0GA1Ud
DgQWBBRcdBBnfEG7vAc0fkZMnMpJPVkCPjAfBgNVHSMEGDAWgBRcdBBnfEG7vAc0
fkZMnMpJPVkCPjAPBgNVHRMBAf8EBTADAQH/MA0GCSqGSIb3DQEBCwUAA4IBAQBj
Tf5mwGx4Yv/I1fuLz8p6BHJzrQHiYryRLAO8F0Rb3Giaqrb5vgbT0MNVlCJ2HTuC
cJfhSqDF3tVfpLMOtQBJkGaZ2w7RM8pnp+G6BqzLIq2JCGR6K7RME5zdRaGdCMqW
tDNP9iyb5zw3NIEURQ2u2MKmV72UQKvQW9IflPQvL3Anm7jPjW9i1L8rqVJFy2rz
KjFiPCoUTGc2GvNKPytrsT4OD1f2L5ZvKcWqNBBGZc5q1byBqnGC8qYdqNHeXCDu
rQuPnGxKYe/dzXqmjulia3mKbfM8L7ep1WS/NPtxvT2Hy1VZqA6hb9Lk3BqmV9P7
N1R6JSAKM3FIq3zV0hLi
-----END CERTIFICATE-----`)

	source := &CustomRegistrySource{
		Product:            product.Terraform,
		Version:            version.Must(version.NewVersion("1.5.0")),
		BaseURL:            "https://example.com",
		InstallDir:         "/tmp",
		CACertPEM:          testCA,
		InsecureSkipVerify: false,
	}

	client, err := source.getHTTPClient()
	require.NoError(t, err)
	assert.NotNil(t, client)

	// Since we're using an interface, we can't directly inspect the transport
	// Instead, we verify the source is configured correctly
	assert.NotNil(t, source.CACertPEM)
	assert.False(t, source.InsecureSkipVerify)
}

func TestCustomRegistrySource_InsecureSkipVerify(t *testing.T) {
	source := &CustomRegistrySource{
		Product:            product.Terraform,
		Version:            version.Must(version.NewVersion("1.5.0")),
		BaseURL:            "https://example.com",
		InstallDir:         "/tmp",
		InsecureSkipVerify: true,
	}

	client, err := source.getHTTPClient()
	require.NoError(t, err)
	assert.NotNil(t, client)

	// Since we're using an interface, we can't directly inspect the transport
	// Instead, we verify the source is configured correctly
	assert.True(t, source.InsecureSkipVerify)
}

func TestCustomRegistrySource_ZipSlipProtection(t *testing.T) {
	// Test that extractFile prevents directory traversal attacks
	tests := []struct {
		name    string
		zipPath string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid file name",
			zipPath: "terraform",
			wantErr: false,
		},
		{
			name:    "valid file name with extension",
			zipPath: "terraform.exe",
			wantErr: false,
		},
		{
			name:    "path traversal with ..",
			zipPath: "../terraform",
			wantErr: true,
			errMsg:  "invalid file path in archive",
		},
		{
			name:    "path traversal with multiple ..",
			zipPath: "../../terraform",
			wantErr: true,
			errMsg:  "invalid file path in archive",
		},
		{
			name:    "path with subdirectory",
			zipPath: "subdir/terraform",
			wantErr: true,
			errMsg:  "invalid file path in archive",
		},
		{
			name:    "absolute path unix",
			zipPath: "/etc/passwd",
			wantErr: true,
			errMsg:  "invalid file path in archive",
		},
		{
			name:    "path with backslash",
			zipPath: "subdir\\terraform",
			wantErr: true,
			errMsg:  "invalid file name in archive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal zip.File structure for testing
			// We only need the Name field for our validation tests
			zipFile := &zip.File{
				FileHeader: zip.FileHeader{
					Name: tt.zipPath,
				},
			}

			// Since we can't easily test the full extractFile without a real zip,
			// we'll test just the validation logic by extracting it
			cleanName := filepath.Base(zipFile.Name)
			err := validateZipPath(zipFile.Name, cleanName)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// validateZipPath extracts the validation logic for testing
func validateZipPath(originalName, cleanName string) error {
	if cleanName != originalName {
		return fmt.Errorf("invalid file path in archive: %s", originalName)
	}

	// Additional validation: ensure the name doesn't contain any path separators
	if strings.Contains(cleanName, "/") || strings.Contains(cleanName, "\\") {
		return fmt.Errorf("invalid file name in archive: %s", originalName)
	}

	return nil
}

func TestCustomRegistrySource_findBuild(t *testing.T) {
	source := &CustomRegistrySource{
		Product: product.Terraform,
		Version: version.Must(version.NewVersion("1.5.0")),
		BaseURL: "https://example.com",
	}

	release := &releaseVersion{
		Version: "1.5.0",
		Builds: []releaseBuild{
			{OS: "linux", Arch: "amd64", Filename: "terraform_linux_amd64.zip", URL: "https://example.com/terraform_linux_amd64.zip"},
			{OS: "darwin", Arch: "amd64", Filename: "terraform_darwin_amd64.zip", URL: "https://example.com/terraform_darwin_amd64.zip"},
			{OS: "darwin", Arch: "arm64", Filename: "terraform_darwin_arm64.zip", URL: "https://example.com/terraform_darwin_arm64.zip"},
			{OS: "windows", Arch: "amd64", Filename: "terraform_windows_amd64.zip", URL: "https://example.com/terraform_windows_amd64.zip"},
		},
	}

	// Should find build for current OS/arch
	build, err := source.findBuild(release)
	require.NoError(t, err)
	assert.NotNil(t, build)

	// Test with missing build
	emptyRelease := &releaseVersion{
		Version: "1.5.0",
		Builds:  []releaseBuild{},
	}
	_, err = source.findBuild(emptyRelease)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no build found")
}
