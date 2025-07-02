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

package tools

import (
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetValidPlatform(t *testing.T) {
	osArchTests := []struct {
		currentOS   string
		currentArch string
		out         string
		err         error
	}{
		{
			currentOS:   "darwin",
			currentArch: "amd64",
			out:         "bicep-osx-x64",
		},
		{
			currentOS:   "darwin",
			currentArch: "arm64",
			out:         "bicep-osx-arm64",
		},
		{
			currentOS:   "windows",
			currentArch: "amd64",
			out:         "bicep-win-x64",
		},
		{
			currentOS:   "windows",
			currentArch: "arm64",
			out:         "bicep-win-arm64",
		},
		{
			currentOS:   "linux",
			currentArch: "amd64",
			out:         "bicep-linux-x64",
		},
		{
			currentOS:   "linux",
			currentArch: "arm",
			out:         "",
			err:         errors.New("unsupported platform linux/arm"),
		},
		{
			currentOS:   "linux",
			currentArch: "arm64",
			out:         "bicep-linux-arm64",
		},
	}

	for _, tc := range osArchTests {
		t.Run(tc.currentOS+"-"+tc.currentArch, func(t *testing.T) {
			platform, err := GetValidPlatform(tc.currentOS, tc.currentArch)
			if tc.err != nil {
				require.ErrorContains(t, err, err.Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.out, platform, "GetValidPlatform() got = %v, want %v", platform, tc.out)
		})
	}
}

func TestValidateDownloadURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantError bool
	}{
		{
			name:      "empty URL is valid",
			url:       "",
			wantError: false,
		},
		{
			name:      "valid HTTPS URL",
			url:       "https://github.com/Azure/bicep/releases",
			wantError: false,
		},
		{
			name:      "HTTP URL is invalid",
			url:       "http://example.com/releases",
			wantError: true,
		},
		{
			name:      "invalid URL format",
			url:       "not-a-url",
			wantError: true,
		},
		{
			name:      "FTP URL is invalid",
			url:       "ftp://example.com/releases",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDownloadURL(tt.url)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConstructDownloadURL(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		version    string
		binaryName string
		expected   string
	}{
		{
			name:       "default bicep URL with no custom params",
			baseURL:    "",
			version:    "",
			binaryName: "bicep-linux-x64",
			expected:   "https://github.com/Azure/bicep/releases/latest/download/bicep-linux-x64",
		},
		{
			name:       "default bicep URL with version",
			baseURL:    "",
			version:    "v0.21.1",
			binaryName: "bicep-linux-x64",
			expected:   "https://github.com/Azure/bicep/releases/download/v0.21.1/bicep-linux-x64",
		},
		{
			name:       "custom base URL with version",
			baseURL:    "https://internal.company.com/bicep/releases",
			version:    "v0.21.1",
			binaryName: "bicep-linux-x64",
			expected:   "https://internal.company.com/bicep/releases/v0.21.1/bicep-linux-x64",
		},
		{
			name:       "custom base URL without version",
			baseURL:    "https://internal.company.com/bicep/releases",
			version:    "",
			binaryName: "bicep-linux-x64",
			expected:   "https://internal.company.com/bicep/releases/bicep-linux-x64",
		},
		{
			name:       "custom base URL with trailing slash",
			baseURL:    "https://internal.company.com/bicep/releases/",
			version:    "v0.21.1",
			binaryName: "bicep-linux-x64",
			expected:   "https://internal.company.com/bicep/releases/v0.21.1/bicep-linux-x64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructDownloadURL(tt.baseURL, tt.version, tt.binaryName)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestConstructManifestDownloadURL(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		version    string
		binaryName string
		expected   string
	}{
		{
			name:       "default manifest URL with no custom params",
			baseURL:    "",
			version:    "",
			binaryName: "manifest-to-bicep-extension-linux-amd64",
			expected:   "https://github.com/willdavsmith/bicep-tools/releases/download/v0.2.0/manifest-to-bicep-extension-linux-amd64",
		},
		{
			name:       "default manifest URL with version",
			baseURL:    "",
			version:    "v0.3.0",
			binaryName: "manifest-to-bicep-extension-linux-amd64",
			expected:   "https://github.com/willdavsmith/bicep-tools/releases/download/v0.3.0/manifest-to-bicep-extension-linux-amd64",
		},
		{
			name:       "custom base URL with version",
			baseURL:    "https://internal.company.com/manifest/releases",
			version:    "v0.3.0",
			binaryName: "manifest-to-bicep-extension-linux-amd64",
			expected:   "https://internal.company.com/manifest/releases/v0.3.0/manifest-to-bicep-extension-linux-amd64",
		},
		{
			name:       "custom base URL without version",
			baseURL:    "https://internal.company.com/manifest/releases",
			version:    "",
			binaryName: "manifest-to-bicep-extension-linux-amd64",
			expected:   "https://internal.company.com/manifest/releases/manifest-to-bicep-extension-linux-amd64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := constructManifestDownloadURL(tt.baseURL, tt.version, tt.binaryName)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFilename(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		expected string
	}{
		{
			name:     "binary without extension",
			base:     "bicep-linux-x64",
			expected: func() string {
				if runtime.GOOS == "windows" {
					return "bicep-linux-x64.exe"
				}
				return "bicep-linux-x64"
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getFilename(tt.base)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestManifestExtensionPlatforms(t *testing.T) {
	// Test that manifest extension platforms contain expected entries
	expectedPlatforms := []string{
		"windows-amd64",
		"linux-amd64",
		"linux-arm64",
		"darwin-amd64",
		"darwin-arm64",
	}

	for _, platform := range expectedPlatforms {
		_, exists := manifestExtensionPlatforms[platform]
		require.True(t, exists, "Expected platform %s to exist in manifestExtensionPlatforms", platform)
	}

	// Test that missing platforms are handled
	_, exists := manifestExtensionPlatforms["linux-arm"]
	require.False(t, exists, "linux-arm should not be supported for manifest extension")
}
