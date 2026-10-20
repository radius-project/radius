/*
Copyright 2025 The Radius Authors.

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

package tfbinary

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCopyExecutable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "global", BinaryName)
	require.NoError(t, os.WriteFile(src, []byte("binary"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0755))

	require.NoError(t, CopyExecutable(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "binary", string(data))

	info, err := os.Stat(dst)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())

	entries, err := os.ReadDir(filepath.Dir(dst))
	require.NoError(t, err)
	require.Len(t, entries, 1, "temporary file should be renamed into place")
}

func TestCopyExecutableReplacesExistingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, BinaryName)
	require.NoError(t, os.WriteFile(src, []byte("new"), 0600))
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0755))

	// Keep the old file open, as a running recipe would, to show the rename does not modify it.
	old, err := os.Open(dst)
	require.NoError(t, err)
	defer old.Close()

	require.NoError(t, CopyExecutable(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "new", string(data))

	oldData := make([]byte, 3)
	_, err = old.Read(oldData)
	require.NoError(t, err)
	require.Equal(t, "old", string(oldData))
}

func TestCopyExecutableMissingSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.Error(t, CopyExecutable(filepath.Join(dir, "missing"), filepath.Join(dir, BinaryName)))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func zipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestDownloadURL(t *testing.T) {
	t.Parallel()

	archive := zipArchive(t, map[string]string{BinaryName: "custom-terraform", "LICENSE.txt": "license"})
	other := zipArchive(t, map[string]string{"bin/terraform": "nested", "README": "readme"})

	tests := []struct {
		name     string
		body     []byte
		status   int
		checksum string
		want     string
		wantErr  string
	}{
		{name: "no checksum", body: archive, status: http.StatusOK, want: "custom-terraform"},
		{name: "matching checksum", body: archive, status: http.StatusOK, checksum: sha256Hex(archive), want: "custom-terraform"},
		{name: "matching uppercase checksum", body: archive, status: http.StatusOK, checksum: strings.ToUpper(sha256Hex(archive)), want: "custom-terraform"},
		{name: "checksum mismatch", body: archive, status: http.StatusOK, checksum: sha256Hex([]byte("other")), wantErr: "checksum mismatch"},
		{name: "http error", body: nil, status: http.StatusNotFound, wantErr: "unexpected status 404"},
		{name: "not a zip", body: []byte("not a zip"), status: http.StatusOK, wantErr: "failed to read Terraform archive"},
		{name: "no top-level binary", body: other, status: http.StatusOK, wantErr: "does not contain a top-level terraform binary"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			defer server.Close()

			dir := t.TempDir()
			dst := filepath.Join(dir, BinaryName)
			err := DownloadURL(context.Background(), server.Client(), server.URL+"/terraform.zip", tt.checksum, dst)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.NoFileExists(t, dst)
				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Empty(t, entries, "no partial files should be left behind")
				return
			}

			require.NoError(t, err)
			data, err := os.ReadFile(dst)
			require.NoError(t, err)
			require.Equal(t, tt.want, string(data))
			info, err := os.Stat(dst)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0755), info.Mode().Perm())
		})
	}
}
