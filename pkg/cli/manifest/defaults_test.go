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

package manifest

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultManifests(t *testing.T) {
	t.Parallel()

	validManifest1 := `
namespace: Radius.Compute
types:
  containers:
    apiVersions:
      "2025-08-01-preview":
        schema: {}`

	validManifest2 := `
namespace: Radius.Compute
types:
  routes:
    apiVersions:
      "2025-08-01-preview":
        schema: {}`

	validManifest3 := `
namespace: Radius.Security
types:
  secrets:
    apiVersions:
      "2025-08-01-preview":
        schema: {}`

	t.Run("merges manifests sharing a namespace", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Compute/containers
  - Radius.Compute/routes
  - Radius.Security/secrets
`),
			},
			"Compute/containers/containers.yaml": &fstest.MapFile{Data: []byte(validManifest1)},
			"Compute/routes/routes.yaml":         &fstest.MapFile{Data: []byte(validManifest2)},
			"Security/secrets/secrets.yaml":       &fstest.MapFile{Data: []byte(validManifest3)},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.NoError(t, err)
		require.Len(t, providers, 2)

		var computeProvider *ResourceProvider
		var securityProvider *ResourceProvider
		for i := range providers {
			switch providers[i].Namespace {
			case "Radius.Compute":
				computeProvider = &providers[i]
			case "Radius.Security":
				securityProvider = &providers[i]
			}
		}

		require.NotNil(t, computeProvider)
		assert.Len(t, computeProvider.Types, 2)
		assert.Contains(t, computeProvider.Types, "containers")
		assert.Contains(t, computeProvider.Types, "routes")

		require.NotNil(t, securityProvider)
		assert.Len(t, securityProvider.Types, 1)
		assert.Contains(t, securityProvider.Types, "secrets")
	})

	t.Run("returns error when defaults.yaml is missing", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read defaults.yaml")
		assert.Nil(t, providers)
	})

	t.Run("returns nil when defaults.yaml has no entries", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte("defaultRegistration:\n"),
			},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.NoError(t, err)
		assert.Nil(t, providers)
	})

	t.Run("returns error when manifest file is missing from FS", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Compute/nonexistent
`),
			},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read manifest for Radius.Compute/nonexistent")
		assert.Nil(t, providers)
	})

	t.Run("returns error for invalid manifest YAML", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Bad/thing
`),
			},
			"Bad/thing/thing.yaml": &fstest.MapFile{Data: []byte("invalid: yaml: [")},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse manifest")
		assert.Nil(t, providers)
	})

	t.Run("returns error for invalid resource type name format", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - InvalidFormat
`),
			},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resource type name")
		assert.Nil(t, providers)
	})

	t.Run("returns error for non-Radius namespace", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Other.Compute/containers
`),
			},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must start with 'Radius.'")
		assert.Nil(t, providers)
	})

	t.Run("returns error when manifest namespace does not match expected", func(t *testing.T) {
		t.Parallel()

		mismatchedManifest := `
namespace: Radius.Other
types:
  containers:
    apiVersions:
      "2025-08-01-preview":
        schema: {}`

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Compute/containers
`),
			},
			"Compute/containers/containers.yaml": &fstest.MapFile{Data: []byte(mismatchedManifest)},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "declares namespace")
		assert.Contains(t, err.Error(), "expected")
		assert.Nil(t, providers)
	})

	t.Run("returns error when manifest does not define expected type", func(t *testing.T) {
		t.Parallel()

		// Manifest declares Radius.Compute namespace but only has 'routes', not 'containers'
		wrongTypeManifest := `
namespace: Radius.Compute
types:
  routes:
    apiVersions:
      "2025-08-01-preview":
        schema: {}`

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Compute/containers
`),
			},
			"Compute/containers/containers.yaml": &fstest.MapFile{Data: []byte(wrongTypeManifest)},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "does not define resource type")
		assert.Contains(t, err.Error(), "containers")
		assert.Nil(t, providers)
	})

	t.Run("returns single provider without merging", func(t *testing.T) {
		t.Parallel()

		fsys := fstest.MapFS{
			"defaults.yaml": &fstest.MapFile{
				Data: []byte(`defaultRegistration:
  - Radius.Security/secrets
`),
			},
			"Security/secrets/secrets.yaml": &fstest.MapFile{Data: []byte(validManifest3)},
		}

		providers, err := LoadDefaultManifests(context.Background(), fsys)
		require.NoError(t, err)
		require.Len(t, providers, 1)
		assert.Equal(t, "Radius.Security", providers[0].Namespace)
		assert.Contains(t, providers[0].Types, "secrets")
	})
}

func TestResolveResourceTypePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:     "standard resource type",
			input:    "Radius.Compute/containers",
			expected: "Compute/containers/containers.yaml",
		},
		{
			name:     "nested namespace",
			input:    "Radius.Security/secrets",
			expected: "Security/secrets/secrets.yaml",
		},
		{
			name:     "data namespace",
			input:    "Radius.Data/mySqlDatabases",
			expected: "Data/mySqlDatabases/mySqlDatabases.yaml",
		},
		{
			name:        "missing slash",
			input:       "Radius.Compute",
			expectError: true,
		},
		{
			name:        "non-Radius namespace",
			input:       "Other.Compute/containers",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := resolveResourceTypePath(tt.input)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
