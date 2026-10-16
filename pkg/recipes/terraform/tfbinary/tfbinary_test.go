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
	"os"
	"path/filepath"
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
