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

func TestParseRequirementsTxt(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantReqs []PythonRequirement
		wantErr  bool
	}{
		{
			name: "basic requirements",
			content: `flask==3.0.0
psycopg2-binary>=2.9.0
redis
`,
			wantReqs: []PythonRequirement{
				{Name: "flask", Version: "==3.0.0"},
				{Name: "psycopg2-binary", Version: ">=2.9.0"},
				{Name: "redis", Version: ""},
			},
			wantErr: false,
		},
		{
			name: "with comments and extras",
			content: `# This is a comment
django>=4.0
celery[redis]>=5.0

# Database
psycopg2>=2.9
`,
			wantReqs: []PythonRequirement{
				{Name: "django", Version: ">=4.0"},
				{Name: "celery", Version: ">=5.0", Extras: []string{"redis"}},
				{Name: "psycopg2", Version: ">=2.9"},
			},
			wantErr: false,
		},
		{
			name: "with options",
			content: `-r base.txt
-e .
flask==3.0.0
`,
			wantReqs: []PythonRequirement{
				{Name: "flask", Version: "==3.0.0"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			reqPath := filepath.Join(tmpDir, "requirements.txt")
			err := os.WriteFile(reqPath, []byte(tt.content), 0644)
			require.NoError(t, err)

			// Parse
			reqs, err := ParseRequirementsTxt(reqPath)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, reqs.Requirements, len(tt.wantReqs))

			for i, want := range tt.wantReqs {
				got := reqs.Requirements[i]
				assert.Equal(t, want.Name, got.Name)
				assert.Equal(t, want.Version, got.Version)
				if len(want.Extras) > 0 {
					assert.Equal(t, want.Extras, got.Extras)
				}
			}
		})
	}
}

func TestRequirements_HasPackage(t *testing.T) {
	reqs := &Requirements{
		Requirements: []PythonRequirement{
			{Name: "flask", Version: "==3.0.0"},
			{Name: "psycopg2-binary", Version: ">=2.9.0"},
		},
	}

	assert.True(t, reqs.HasPackage("flask"))
	assert.True(t, reqs.HasPackage("psycopg2-binary"))
	assert.False(t, reqs.HasPackage("django"))
}

func TestRequirements_AllPackages(t *testing.T) {
	reqs := &Requirements{
		Requirements: []PythonRequirement{
			{Name: "flask", Version: "==3.0.0"},
			{Name: "redis", Version: ""},
		},
	}

	pkgs := reqs.AllPackages()
	assert.Len(t, pkgs, 2)
	assert.Equal(t, "==3.0.0", pkgs["flask"])
	assert.Equal(t, "", pkgs["redis"])
}

func TestFindRequirementsTxt(t *testing.T) {
	tmpDir := t.TempDir()

	// Create requirements files
	err := os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte("flask"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "requirements-dev.txt"), []byte("pytest"), 0644)
	require.NoError(t, err)

	// Create venv (should be skipped)
	venvDir := filepath.Join(tmpDir, "venv")
	err = os.MkdirAll(venvDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(venvDir, "requirements.txt"), []byte("pip"), 0644)
	require.NoError(t, err)

	files, err := FindRequirementsTxt(tmpDir)
	require.NoError(t, err)

	// Should find 2 files (requirements.txt + requirements-dev.txt), not venv
	assert.Len(t, files, 2)
}
