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

func TestParseGoMod(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantModule  string
		wantVersion string
		wantDeps    int
		wantErr     bool
	}{
		{
			name: "basic go.mod",
			content: `module github.com/example/myapp

go 1.22

require (
	github.com/lib/pq v1.10.9
	github.com/redis/go-redis/v9 v9.3.0
)
`,
			wantModule:  "github.com/example/myapp",
			wantVersion: "1.22",
			wantDeps:    2,
			wantErr:     false,
		},
		{
			name: "with indirect deps",
			content: `module example.com/app

go 1.21

require (
	github.com/jackc/pgx/v5 v5.5.0
	golang.org/x/net v0.19.0 // indirect
)
`,
			wantModule:  "example.com/app",
			wantVersion: "1.21",
			wantDeps:    2,
			wantErr:     false,
		},
		{
			name: "single line requires",
			content: `module myapp

go 1.20

require github.com/lib/pq v1.10.0
require github.com/gomodule/redigo v1.8.9
`,
			wantModule:  "myapp",
			wantVersion: "1.20",
			wantDeps:    2,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			modPath := filepath.Join(tmpDir, "go.mod")
			err := os.WriteFile(modPath, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Parse
			mod, err := ParseGoMod(modPath)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantModule, mod.Module)
			assert.Equal(t, tt.wantVersion, mod.GoVersion)
			assert.Len(t, mod.Require, tt.wantDeps)
		})
	}
}

func TestGoModule_DirectDependencies(t *testing.T) {
	mod := &GoModule{
		Require: []GoRequire{
			{Path: "github.com/lib/pq", Version: "v1.10.9", Indirect: false},
			{Path: "golang.org/x/net", Version: "v0.19.0", Indirect: true},
		},
	}

	direct := mod.DirectDependencies()
	assert.Len(t, direct, 1)
	assert.Equal(t, "github.com/lib/pq", direct[0].Path)
}

func TestGoModule_HasDependency(t *testing.T) {
	mod := &GoModule{
		Require: []GoRequire{
			{Path: "github.com/lib/pq", Version: "v1.10.9"},
			{Path: "github.com/redis/go-redis/v9", Version: "v9.3.0"},
		},
	}

	assert.True(t, mod.HasDependency("github.com/lib/pq"))
	assert.True(t, mod.HasDependency("github.com/redis/go-redis"))
	assert.False(t, mod.HasDependency("github.com/example/unknown"))
}

func TestFindGoMod(t *testing.T) {
	tmpDir := t.TempDir()

	// Create go.mod files
	err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module test"), 0644)
	require.NoError(t, err)

	subDir := filepath.Join(tmpDir, "cmd", "api")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(subDir, "go.mod"), []byte("module test/api"), 0644)
	require.NoError(t, err)

	// Create vendor (should be skipped)
	vendorDir := filepath.Join(tmpDir, "vendor")
	err = os.MkdirAll(vendorDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(vendorDir, "go.mod"), []byte("module vendor"), 0644)
	require.NoError(t, err)

	files, err := FindGoMod(tmpDir)
	require.NoError(t, err)

	// Should find 2 files (root + cmd/api), not vendor
	assert.Len(t, files, 2)
}
