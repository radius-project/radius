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

package pgbackup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_HasBackup_AllFilesPresent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, db := range databases {
		f := filepath.Join(dir, db+".sql")
		err := os.WriteFile(f, []byte("-- sql"), 0644)
		require.NoError(t, err)
	}

	require.True(t, HasBackup(dir))
}

func Test_HasBackup_MissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write only the first database file; leave the rest missing.
	f := filepath.Join(dir, databases[0]+".sql")
	err := os.WriteFile(f, []byte("-- sql"), 0644)
	require.NoError(t, err)

	require.False(t, HasBackup(dir))
}

func Test_HasBackup_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.False(t, HasBackup(dir))
}

func Test_HasBackup_NonexistentDir(t *testing.T) {
	t.Parallel()

	require.False(t, HasBackup("/nonexistent/path/that/does/not/exist"))
}

func Test_Constants(t *testing.T) {
	require.Equal(t, "radius-system", DefaultNamespace)
	require.Equal(t, "app.kubernetes.io/name=database", PodLabelSelector)
	require.Equal(t, "radius", PostgresUser)
}

func Test_Databases(t *testing.T) {
	require.Contains(t, databases, "ucp")
	require.Contains(t, databases, "applications_rp")
	require.Contains(t, databases, "dynamic_rp")
	require.Len(t, databases, 3)
}
