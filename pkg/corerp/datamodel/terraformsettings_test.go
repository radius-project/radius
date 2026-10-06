/*
Copyright 2026 The Radius Authors.

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

package datamodel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerraformBackendValidate(t *testing.T) {
	tests := []struct {
		name    string
		backend *TerraformBackend
		valid   bool
	}{
		{"default Kubernetes", nil, true},
		{"s3", &TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", KeyPrefix: "radius"}, true},
		{"azure", &TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius", KeyPrefix: "radius"}, true},
		{"unsupported", &TerraformBackend{Type: "local", KeyPrefix: "radius"}, false},
		{"missing bucket", &TerraformBackend{Type: "s3", Region: "us-west-2", KeyPrefix: "radius"}, false},
		{"missing region", &TerraformBackend{Type: "s3", Bucket: "states", KeyPrefix: "radius"}, false},
		{"missing account", &TerraformBackend{Type: "azurerm", ContainerName: "radius", KeyPrefix: "radius"}, false},
		{"missing container", &TerraformBackend{Type: "azurerm", StorageAccountName: "states", KeyPrefix: "radius"}, false},
		{"missing key prefix s3", &TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"}, false},
		{"missing key prefix azure", &TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"}, false},
		{"cross cloud s3", &TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", ContainerName: "radius", KeyPrefix: "radius"}, false},
		{"cross cloud azure", &TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius", Bucket: "states", KeyPrefix: "radius"}, false},
		{"whitespace", &TerraformBackend{Type: "s3", Bucket: " states", Region: "us-west-2", KeyPrefix: "radius"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.valid {
				require.NoError(t, tt.backend.Validate())
			} else {
				require.Error(t, tt.backend.Validate())
			}
		})
	}
	for _, prefix := range []string{"", "/radius", "radius/", "a//b", "a/../b", ".", "a b", "a\\b", strings.Repeat("a", 969)} {
		t.Run("invalid prefix "+prefix[:min(len(prefix), 20)], func(t *testing.T) {
			require.Error(t, (&TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", KeyPrefix: prefix}).Validate())
		})
	}
	for _, prefix := range []string{"radius", "install_1/team-a", strings.Repeat("a", 968)} {
		require.NoError(t, (&TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", KeyPrefix: prefix}).Validate())
	}

	// Both names are interpolated into the blob endpoint URL used for state cleanup, so characters
	// that could alter that URL's authority or path must be rejected outright.
	for _, name := range []string{"ab", "States", "state-s", "state_s", "states/evil", "states@evil", strings.Repeat("a", 25)} {
		t.Run("invalid account "+name[:min(len(name), 20)], func(t *testing.T) {
			require.ErrorContains(t, (&TerraformBackend{Type: "azurerm", StorageAccountName: name, ContainerName: "radius", KeyPrefix: "radius"}).Validate(), "backend.storageAccountName")
		})
	}
	for _, name := range []string{"ab", "Radius", "-radius", "radius-", "radius/evil", "radius@evil", strings.Repeat("a", 64)} {
		t.Run("invalid container "+name[:min(len(name), 20)], func(t *testing.T) {
			require.ErrorContains(t, (&TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: name, KeyPrefix: "radius"}).Validate(), "backend.containerName")
		})
	}
	for _, name := range []string{"abc", "states1", strings.Repeat("a", 24)} {
		require.NoError(t, (&TerraformBackend{Type: "azurerm", StorageAccountName: name, ContainerName: "radius-state1", KeyPrefix: "radius"}).Validate())
	}
	for _, name := range []string{"abc", "radius-state-1", strings.Repeat("a", 63)} {
		require.NoError(t, (&TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: name, KeyPrefix: "radius"}).Validate())
	}
}
