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
		{"s3", &TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2"}, true},
		{"azure", &TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius"}, true},
		{"unsupported", &TerraformBackend{Type: "local"}, false},
		{"missing bucket", &TerraformBackend{Type: "s3", Region: "us-west-2"}, false},
		{"missing region", &TerraformBackend{Type: "s3", Bucket: "states"}, false},
		{"missing account", &TerraformBackend{Type: "azurerm", ContainerName: "radius"}, false},
		{"missing container", &TerraformBackend{Type: "azurerm", StorageAccountName: "states"}, false},
		{"cross cloud s3", &TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", ContainerName: "radius"}, false},
		{"cross cloud azure", &TerraformBackend{Type: "azurerm", StorageAccountName: "states", ContainerName: "radius", Bucket: "states"}, false},
		{"whitespace", &TerraformBackend{Type: "s3", Bucket: " states", Region: "us-west-2"}, false},
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
			require.Error(t, (&TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", KeyPrefix: &prefix}).Validate())
		})
	}
	for _, prefix := range []string{"radius", "install_1/team-a", strings.Repeat("a", 968)} {
		require.NoError(t, (&TerraformBackend{Type: "s3", Bucket: "states", Region: "us-west-2", KeyPrefix: &prefix}).Validate())
	}
}
