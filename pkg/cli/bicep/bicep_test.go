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

package bicep

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetryDownload(t *testing.T) {
	tests := []struct {
		name         string
		toolName     string
		downloadFunc func() error
		expectError  bool
		errorMessage string
	}{
		{
			name:     "successful download on first attempt",
			toolName: "test-tool",
			downloadFunc: func() error {
				return nil
			},
			expectError: false,
		},
		{
			name:     "successful download on second attempt",
			toolName: "test-tool",
			downloadFunc: func() func() error {
				callCount := 0
				return func() error {
					callCount++
					if callCount == 1 {
						return errors.New("temporary failure")
					}
					return nil
				}
			}(),
			expectError: false,
		},
		{
			name:     "failure after all retries",
			toolName: "test-tool",
			downloadFunc: func() error {
				return errors.New("persistent failure")
			},
			expectError:  true,
			errorMessage: "failed to download test-tool after 10 attempts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := retryDownload(tt.toolName, tt.downloadFunc)
			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMessage)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDownloadOptions(t *testing.T) {
	// Test that DownloadOptions struct can be created with all fields
	options := DownloadOptions{
		BicepURL:                         "https://custom.example.com/bicep",
		BicepVersion:                     "v0.21.1",
		ManifestToBicepExtensionURL:      "https://custom.example.com/manifest",
		ManifestToBicepExtensionVersion:  "v0.3.0",
	}

	require.Equal(t, "https://custom.example.com/bicep", options.BicepURL)
	require.Equal(t, "v0.21.1", options.BicepVersion)
	require.Equal(t, "https://custom.example.com/manifest", options.ManifestToBicepExtensionURL)
	require.Equal(t, "v0.3.0", options.ManifestToBicepExtensionVersion)
}

func TestDownloadBicep(t *testing.T) {
	// This is a basic test to ensure DownloadBicep calls DownloadBicepTools
	// We can't fully test without mocking the actual download, but we can verify
	// the function exists and has the correct signature
	require.NotNil(t, DownloadBicep)
}

func TestConstants(t *testing.T) {
	// Test that all required constants are defined
	require.Equal(t, "RAD_BICEP", radBicepEnvVar)
	require.Equal(t, "RAD_MANIFEST_TO_BICEP_EXTENSION", radManifestToBicepExtensionEnvVar)
	require.Equal(t, "rad-bicep", binaryName)
	require.Equal(t, "manifest-to-bicep-extension", manifestToBicepExtensionBinaryName)
	require.Equal(t, 10, retryAttempts)
	require.Equal(t, 5, retryDelaySecs)
}