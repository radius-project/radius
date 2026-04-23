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

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git command %v failed: %s", args, string(out))
	return string(out)
}

// initTestRepo creates a bare-minimum git repo in a temp dir with one commit
// so that worktree operations work. Returns the repo root path and a cleanup func.
func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.name", "test"},
		{"git", "config", "user.email", "test@test.com"},
	}
	for _, args := range cmds {
		runGit(t, dir, args...)
	}

	// We need at least one commit on a branch for worktree operations.
	f := filepath.Join(dir, "README.md")
	require.NoError(t, os.WriteFile(f, []byte("test"), 0o644))

	for _, args := range [][]string{
		{"git", "add", "-A"},
		{"git", "commit", "-m", "init"},
	} {
		runGit(t, dir, args...)
	}

	return dir
}

func initTestRepoWithOrigin(t *testing.T) (string, string) {
	t.Helper()

	repoDir := initTestRepo(t)
	remoteParent := t.TempDir()
	remoteDir := filepath.Join(remoteParent, "origin.git")
	runGit(t, remoteParent, "git", "init", "--bare", remoteDir)
	runGit(t, repoDir, "git", "remote", "add", "origin", remoteDir)
	runGit(t, repoDir, "git", "push", "-u", "origin", "HEAD")

	return repoDir, remoteDir
}

func cloneRepo(t *testing.T, remoteDir string) string {
	t.Helper()

	cloneDir := filepath.Join(t.TempDir(), "clone")
	runGit(t, filepath.Dir(cloneDir), "git", "clone", remoteDir, cloneDir)
	return cloneDir
}

func TestOpenOrCreate_CreatesOrphanBranch(t *testing.T) {
	repoDir := initTestRepo(t)

	// Override working directory so repoRoot() finds our test repo.
	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	// Worktree path should exist.
	info, err := os.Stat(wt.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	// Branch should now exist.
	require.True(t, branchExists(ctx, repoDir, branchName))
}

func TestOpenOrCreate_ReusesBranch(t *testing.T) {
	repoDir := initTestRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	// First open creates the branch.
	wt1, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	wt1.Remove(ctx)

	// Second open reuses the existing branch.
	wt2, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt2.Remove(ctx)

	require.True(t, branchExists(ctx, repoDir, branchName))
}

func TestOpenOrCreate_UsesRemoteBranch(t *testing.T) {
	sourceDir, remoteDir := initTestRepoWithOrigin(t)
	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(sourceDir))

	seedWorktree, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	require.NoError(t, seedWorktree.WriteFile("main/app.json", []byte(`{"version":"seed"}`)))
	require.NoError(t, seedWorktree.CommitAndPush(ctx, "test: seed remote graph artifact"))
	seedWorktree.Remove(ctx)
	require.NoError(t, os.Chdir(origDir))

	cloneDir := cloneRepo(t, remoteDir)
	require.NoError(t, os.Chdir(cloneDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	data, err := os.ReadFile(filepath.Join(wt.Path, "main", "app.json"))
	require.NoError(t, err)
	require.Equal(t, `{"version":"seed"}`, string(data))
}

func TestWriteFile_CreatesDirectories(t *testing.T) {
	repoDir := initTestRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	err = wt.WriteFile("main/app.json", []byte(`{"version":"1.0.0"}`))
	require.NoError(t, err)

	// File should exist at the expected path.
	data, err := os.ReadFile(filepath.Join(wt.Path, "main", "app.json"))
	require.NoError(t, err)
	require.Equal(t, `{"version":"1.0.0"}`, string(data))
}

func TestCommitAndPush_CommitsChanges(t *testing.T) {
	repoDir := initTestRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	require.NoError(t, wt.WriteFile("feature/app.json", []byte(`{"version":"1.0.0"}`)))

	// CommitAndPush will commit; push will fail (no remote) but that's non-fatal.
	err = wt.CommitAndPush(ctx, "test: add graph artifact")
	require.NoError(t, err)

	// Verify the commit exists on the branch by checking git log.
	cmd := exec.Command("git", "log", "--oneline", branchName, "--", "feature/app.json")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "test: add graph artifact")
}

func TestCommitAndPush_NoChanges(t *testing.T) {
	repoDir := initTestRepo(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	// CommitAndPush with no changes should succeed (no-op).
	err = wt.CommitAndPush(ctx, "test: empty")
	require.NoError(t, err)
}

func TestCommitAndPush_ReturnsErrorWhenOriginPushFails(t *testing.T) {
	repoDir, _ := initTestRepoWithOrigin(t)

	origDir, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(repoDir))
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	require.NoError(t, wt.WriteFile("feature/app.json", []byte(`{"version":"1.0.0"}`)))
	runGit(t, repoDir, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

	err = wt.CommitAndPush(ctx, "test: push should fail")
	require.Error(t, err)
	require.Contains(t, err.Error(), "pushing state branch")
}
