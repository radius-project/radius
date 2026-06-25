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

// newTestRepo creates a throwaway git repository with one commit on the main branch and returns
// its root. All git operations in gitstate run relative to a checkout, so tests need a real repo.
func newTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v failed: %s", args, out)
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("test"), 0o644))
	run("add", "-A")
	run("commit", "-m", "initial")

	return root
}

// inDir runs fn with the process working directory temporarily set to dir. gitstate derives the
// repo root from the current directory, so tests must run from inside the repo.
func inDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() {
		require.NoError(t, os.Chdir(orig))
	}()
	fn()
}

func Test_BranchName_DefaultAndOverride(t *testing.T) {
	require.Equal(t, DefaultBranch, BranchName())

	t.Setenv(branchEnvVar, "radius-state-test-123")
	require.Equal(t, "radius-state-test-123", BranchName())
}

func Test_OpenOrCreate_CreatesOrphanBranchAndIsolatesState(t *testing.T) {
	root := newTestRepo(t)

	inDir(t, root, func() {
		ctx := context.Background()
		wt, err := OpenOrCreate(ctx, "radius-state-test")
		require.NoError(t, err)
		defer wt.Remove(ctx)

		// The worktree path is outside the application checkout.
		require.NotEmpty(t, wt.Path)
		require.NoDirExists(t, filepath.Join(root, "radius-state-test"))

		// Writing a state file and committing must not touch the application working tree.
		require.NoError(t, os.WriteFile(filepath.Join(wt.Path, "ucp.sql"), []byte("dump"), 0o644))
		require.NoError(t, wt.CommitAndPush(ctx, "radius: backup"))

		status, err := exec.Command("git", "-C", root, "status", "--porcelain").CombinedOutput()
		require.NoError(t, err)
		require.Empty(t, string(status), "application working tree must stay clean")
	})
}

func Test_OpenOrCreate_RestoresPreviousState(t *testing.T) {
	root := newTestRepo(t)

	inDir(t, root, func() {
		ctx := context.Background()

		// First session writes state and commits it.
		wt1, err := OpenOrCreate(ctx, "radius-state-test")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(wt1.Path, "ucp.sql"), []byte("first-dump"), 0o644))
		require.NoError(t, wt1.CommitAndPush(ctx, "radius: backup 1"))
		wt1.Remove(ctx)

		// Second session must see the committed state.
		wt2, err := OpenOrCreate(ctx, "radius-state-test")
		require.NoError(t, err)
		defer wt2.Remove(ctx)

		data, err := os.ReadFile(filepath.Join(wt2.Path, "ucp.sql"))
		require.NoError(t, err)
		require.Equal(t, "first-dump", string(data))
	})
}

func Test_CommitAndPush_NoRemoteIsNotAnError(t *testing.T) {
	root := newTestRepo(t)

	inDir(t, root, func() {
		ctx := context.Background()
		wt, err := OpenOrCreate(ctx, "radius-state-test")
		require.NoError(t, err)
		defer wt.Remove(ctx)

		require.NoError(t, os.WriteFile(filepath.Join(wt.Path, "ucp.sql"), []byte("dump"), 0o644))
		// No "origin" remote is configured; commit succeeds and the missing remote is tolerated.
		require.NoError(t, wt.CommitAndPush(ctx, "radius: backup"))
	})
}

func Test_CommitAndPush_PushesToRemote(t *testing.T) {
	// Bare remote that the working repo pushes to.
	remote := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", remote)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git init --bare failed: %s", out)

	root := newTestRepo(t)
	addRemote := exec.Command("git", "-C", root, "remote", "add", "origin", remote)
	out, err = addRemote.CombinedOutput()
	require.NoErrorf(t, err, "git remote add failed: %s", out)

	inDir(t, root, func() {
		ctx := context.Background()
		wt, err := OpenOrCreate(ctx, "radius-state-test")
		require.NoError(t, err)
		defer wt.Remove(ctx)

		require.NoError(t, os.WriteFile(filepath.Join(wt.Path, "ucp.sql"), []byte("dump"), 0o644))
		require.NoError(t, wt.CommitAndPush(ctx, "radius: backup"))

		// The branch must now exist on the remote.
		lsRemote, err := exec.Command("git", "-C", root, "ls-remote", "origin", "radius-state-test").CombinedOutput()
		require.NoError(t, err)
		require.Contains(t, string(lsRemote), "radius-state-test")
	})
}
