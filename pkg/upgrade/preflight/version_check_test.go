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

package preflight

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionCompatibilityCheck_Run(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		targetVersion  string
		expectSuccess  bool
		expectMessage  string
	}{
		{
			name:           "valid upgrade to next minor version",
			currentVersion: "v0.43.0",
			targetVersion:  "v0.44.0",
			expectSuccess:  true,
			expectMessage:  "Upgrade from v0.43.0 to v0.44.0 is valid",
		},
		{
			name:           "latest version must be resolved",
			currentVersion: "v0.43.0",
			targetVersion:  "latest",
			expectSuccess:  false,
			expectMessage:  "Target version 'latest' must be resolved to a specific version before validation",
		},
		{
			name:           "invalid downgrade",
			currentVersion: "v0.44.0",
			targetVersion:  "v0.43.0",
			expectSuccess:  false,
			expectMessage:  "Downgrading is not supported",
		},
		{
			name:           "same version",
			currentVersion: "v0.43.0",
			targetVersion:  "v0.43.0",
			expectSuccess:  false,
			expectMessage:  "Target version is the same as current version",
		},
		{
			name:           "skip multiple versions",
			currentVersion: "v0.40.0",
			targetVersion:  "v0.44.0",
			expectSuccess:  false,
			expectMessage:  "Only incremental version upgrades are supported. Expected next version: 0.41.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := NewVersionCompatibilityCheck(tt.currentVersion, tt.targetVersion)

			success, message, err := check.Run(context.Background())

			require.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, success)
			assert.Contains(t, message, tt.expectMessage)
		})
	}
}

func TestVersionCompatibilityCheck_Properties(t *testing.T) {
	check := NewVersionCompatibilityCheck("v0.43.0", "v0.44.0")

	assert.Equal(t, "Version Compatibility", check.Name())
	assert.Equal(t, SeverityError, check.Severity())
}

func TestValidateVersionJump(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		targetVersion  string
		expectValid    bool
		expectMessage  string
	}{
		{
			name:           "safe incremental upgrade",
			currentVersion: "v0.43.0",
			targetVersion:  "v0.44.0",
			expectValid:    true,
		},
		{
			name:           "unsafe version skip",
			currentVersion: "v0.42.0",
			targetVersion:  "v0.46.0",
			expectValid:    false,
			expectMessage:  "Only incremental version upgrades are supported",
		},
		{
			name:           "downgrade attempt",
			currentVersion: "v0.44.0",
			targetVersion:  "v0.43.0",
			expectValid:    false,
			expectMessage:  "Downgrading is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, message, err := ValidateVersionJump(tt.currentVersion, tt.targetVersion)

			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, valid)
			if tt.expectMessage != "" {
				assert.Contains(t, message, tt.expectMessage)
			}
		})
	}
}
