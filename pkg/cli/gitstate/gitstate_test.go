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

package gitstate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// initGitRepo initialises a temporary git repository with one empty commit,
// changes the working directory into it, and restores the original working
// directory when the test finishes.
func initGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}

	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@example.com")
	run("git", "config", "user.name", "Test")
	run("git", "commit", "--allow-empty", "-m", "initial commit")

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })

	require.NoError(t, os.Chdir(dir))
	return dir
}

func Test_OrphanBranchExists_False(t *testing.T) {
	initGitRepo(t)

	require.False(t, OrphanBranchExists(context.Background(), "radius-state"))
}

func Test_OrphanBranchExists_True(t *testing.T) {
	dir := initGitRepo(t)

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
	}

	run("git", "checkout", "--orphan", "radius-state")
	run("git", "commit", "--allow-empty", "-m", "init orphan")
	run("git", "checkout", "main")

	require.True(t, OrphanBranchExists(context.Background(), "radius-state"))
}

func Test_CommitState_CreatesOrphanBranch(t *testing.T) {
	initGitRepo(t) // chdir into repo

	// State files are untracked on main — no stash needed, they survive branch switches.
	stateDir := filepath.Join(".radius", "state")
	require.NoError(t, os.MkdirAll(stateDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "ucp.sql"), []byte("-- ucp"), 0644))

	err := CommitState(context.Background(), stateDir, DefaultBranch)
	require.NoError(t, err)

	// The orphan branch should now exist.
	require.True(t, OrphanBranchExists(context.Background(), DefaultBranch))

	// We should be back on the original branch.
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, runErr := cmd.CombinedOutput()
	require.NoError(t, runErr, string(out))
	require.Equal(t, "main\n", string(out))
}

func Test_RestoreState_NoOpWhenBranchMissing(t *testing.T) {
	initGitRepo(t)

	err := RestoreState(context.Background(), ".radius/state", "radius-state")
	require.NoError(t, err)
}

func Test_CommitAndRestoreState_RoundTrip(t *testing.T) {
	initGitRepo(t) // chdir into repo

	stateDir := filepath.Join(".radius", "state")
	require.NoError(t, os.MkdirAll(stateDir, 0755))

	original := []byte("-- original sql content")
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, "ucp.sql"), original, 0644))

	// Commit state to orphan branch. We come back to main afterwards.
	err := CommitState(context.Background(), stateDir, DefaultBranch)
	require.NoError(t, err)

	// Clear the state directory to simulate a fresh runner checkout.
	require.NoError(t, os.RemoveAll(stateDir))

	// Restore state from orphan branch.
	err = RestoreState(context.Background(), stateDir, DefaultBranch)
	require.NoError(t, err)

	// File should be present and match the original content.
	got, err := os.ReadFile(filepath.Join(stateDir, "ucp.sql"))
	require.NoError(t, err)
	require.Equal(t, original, got)
}

func Test_DefaultBranch_Constant(t *testing.T) {
	require.Equal(t, "radius-state", DefaultBranch)
}
