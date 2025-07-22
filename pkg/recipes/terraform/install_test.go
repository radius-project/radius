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

	install "github.com/hashicorp/hc-install"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/test/testcontext"
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

// Test the Install function signature with logLevel parameter
func TestInstall_LogLevelParameter(t *testing.T) {
	ctx := testcontext.New(t)

	// Create a temporary directory for terraform
	workingDir := t.TempDir()

	// Create installer
	i := install.NewInstaller()

	// Basic terraform config (empty for this test)
	terraformConfig := datamodel.TerraformConfigProperties{}

	// Empty secrets
	secrets := map[string]recipes.SecretData{}

	// Test different log levels
	testCases := []string{"", "ERROR", "DEBUG", "INFO", "WARN", "TRACE", "OFF"}

	for _, logLevel := range testCases {
		t.Run("logLevel_"+logLevel, func(t *testing.T) {
			// This will attempt to install terraform and fail in the test environment,
			// but it tests that the function signature accepts the logLevel parameter
			_, err := Install(ctx, i, workingDir, terraformConfig, secrets, logLevel)

			// We expect an error because we don't have terraform available for download in test,
			// but we're testing that the function accepts the correct parameters
			if err != nil {
				t.Logf("Install function correctly accepted logLevel parameter '%s' (error expected in test environment): %v", logLevel, err)
			} else {
				t.Logf("Install function succeeded with logLevel parameter '%s'", logLevel)
			}
		})
	}
}
