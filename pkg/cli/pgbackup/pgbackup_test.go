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

func writeDumps(t *testing.T, dir string, dbs ...string) {
	t.Helper()
	for _, db := range dbs {
		require.NoError(t, os.WriteFile(filepath.Join(dir, db+".sql"), []byte("-- dump"), 0o644))
	}
}

func Test_HasBackup_AllDumpsPresent(t *testing.T) {
	dir := t.TempDir()
	writeDumps(t, dir, Databases...)

	require.True(t, HasBackup(dir), "HasBackup should be true when every database dump exists")
}

func Test_HasBackup_EmptyDirectory(t *testing.T) {
	require.False(t, HasBackup(t.TempDir()), "an empty directory is not a backup")
}

func Test_HasBackup_PartialDumpsAreNotABackup(t *testing.T) {
	dir := t.TempDir()
	// Only the first database has a dump; the others are missing.
	writeDumps(t, dir, Databases[0])

	require.False(t, HasBackup(dir), "a partial set of dumps must not be treated as a complete backup")
}

func Test_HasBackup_MissingDirectory(t *testing.T) {
	require.False(t, HasBackup(filepath.Join(t.TempDir(), "does-not-exist")))
}

func Test_StateBranchName_DefaultsWhenUnset(t *testing.T) {
	// t.Setenv unsets after the test; explicitly clear to isolate from the ambient environment.
	t.Setenv(StateBranchEnvVar, "")
	require.NoError(t, os.Unsetenv(StateBranchEnvVar))

	require.Equal(t, DefaultStateBranch, StateBranchName(), "an unset override must fall back to the default branch")
}

func Test_StateBranchName_HonorsOverride(t *testing.T) {
	t.Setenv(StateBranchEnvVar, "radius-state-pr-42")
	require.Equal(t, "radius-state-pr-42", StateBranchName(), "%s must override the default branch", StateBranchEnvVar)
}
