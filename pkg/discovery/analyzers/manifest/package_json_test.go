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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePackageJSON(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantName    string
		wantVersion string
		wantDeps    map[string]string
		wantDevDeps map[string]string
		wantErr     bool
	}{
		{
			name: "basic package with dependencies",
			content: `{
				"name": "my-app",
				"version": "1.0.0",
				"dependencies": {
					"pg": "^8.11.3",
					"express": "^4.18.2"
				},
				"devDependencies": {
					"jest": "^29.7.0"
				}
			}`,
			wantName:    "my-app",
			wantVersion: "1.0.0",
			wantDeps:    map[string]string{"pg": "^8.11.3", "express": "^4.18.2"},
			wantDevDeps: map[string]string{"jest": "^29.7.0"},
			wantErr:     false,
		},
		{
			name: "package with no dependencies",
			content: `{
				"name": "empty-app",
				"version": "0.1.0"
			}`,
			wantName:    "empty-app",
			wantVersion: "0.1.0",
			wantDeps:    nil,
			wantDevDeps: nil,
			wantErr:     false,
		},
		{
			name:    "invalid JSON",
			content: `{invalid json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			pkgPath := filepath.Join(tmpDir, "package.json")
			err := os.WriteFile(pkgPath, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Parse
			pkg, err := ParsePackageJSON(pkgPath)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, pkg.Name)
			assert.Equal(t, tt.wantVersion, pkg.Version)
			assert.Equal(t, tt.wantDeps, pkg.Dependencies)
			assert.Equal(t, tt.wantDevDeps, pkg.DevDependencies)
		})
	}
}

func TestPackageJSON_AllDependencies(t *testing.T) {
	pkg := &PackageJSON{
		Dependencies:    map[string]string{"pg": "^8.0.0"},
		DevDependencies: map[string]string{"jest": "^29.0.0"},
	}

	// Without dev deps
	deps := pkg.AllDependencies(false)
	assert.Len(t, deps, 1)
	assert.Equal(t, "^8.0.0", deps["pg"])

	// With dev deps
	allDeps := pkg.AllDependencies(true)
	assert.Len(t, allDeps, 2)
	assert.Equal(t, "^8.0.0", allDeps["pg"])
	assert.Equal(t, "^29.0.0", allDeps["jest"])
}

func TestPackageJSON_HasDependency(t *testing.T) {
	pkg := &PackageJSON{
		Dependencies:    map[string]string{"pg": "^8.0.0"},
		DevDependencies: map[string]string{"jest": "^29.0.0"},
	}

	assert.True(t, pkg.HasDependency("pg"))
	assert.True(t, pkg.HasDependency("jest"))
	assert.False(t, pkg.HasDependency("redis"))
}

func TestFindPackageJSON(t *testing.T) {
	tmpDir := t.TempDir()

	// Create package.json files
	err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{}`), 0644)
	require.NoError(t, err)

	subDir := filepath.Join(tmpDir, "packages", "api")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(subDir, "package.json"), []byte(`{}`), 0644)
	require.NoError(t, err)

	// Create node_modules (should be skipped)
	nmDir := filepath.Join(tmpDir, "node_modules")
	err = os.MkdirAll(nmDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(nmDir, "package.json"), []byte(`{}`), 0644)
	require.NoError(t, err)

	files, err := FindPackageJSON(tmpDir)
	require.NoError(t, err)

	// Should find 2 files (root + packages/api), not node_modules
	assert.Len(t, files, 2)
}
