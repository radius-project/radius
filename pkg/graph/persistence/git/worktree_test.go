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

package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// runGit executes a command (typically "git ...") in dir and asserts success.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %v failed: %s", args, string(out))
	return string(out)
}

// initTestRepo creates a bare-minimum git repo in a temp dir with one commit
// so that worktree operations work.
func initTestRepo(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping git-backed test in -short mode")
	}

	dir := t.TempDir()

	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.name", "test"},
		{"git", "config", "user.email", "test@test.com"},
	} {
		runGit(t, dir, args...)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644))

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

// chdir switches to dir and registers a cleanup to switch back. Returns the
// previous working directory.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestOpenOrCreate_CreatesOrphanBranch(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	info, err := os.Stat(wt.Path)
	require.NoError(t, err)
	require.True(t, info.IsDir())

	require.True(t, branchExists(ctx, repoDir, branchName))
}

func TestOpenOrCreate_ReusesBranch(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt1, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	wt1.Remove(ctx)

	wt2, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt2.Remove(ctx)

	require.True(t, branchExists(ctx, repoDir, branchName))
}

func TestOpenOrCreate_UsesRemoteBranch(t *testing.T) {
	sourceDir, remoteDir := initTestRepoWithOrigin(t)
	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	chdir(t, sourceDir)
	seed, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	require.NoError(t, seed.WriteFile("main/app.json", []byte(`{"version":"seed"}`)))
	require.NoError(t, seed.CommitAndPush(ctx, "test: seed remote graph artifact"))
	seed.Remove(ctx)

	cloneDir := cloneRepo(t, remoteDir)
	chdir(t, cloneDir)

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	data, err := os.ReadFile(filepath.Join(wt.Path, "main", "app.json"))
	require.NoError(t, err)
	require.Equal(t, `{"version":"seed"}`, string(data))
}

func TestWriteFile_CreatesDirectories(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	require.NoError(t, wt.WriteFile("main/app.json", []byte(`{"version":"1.0.0"}`)))

	data, err := os.ReadFile(filepath.Join(wt.Path, "main", "app.json"))
	require.NoError(t, err)
	require.Equal(t, `{"version":"1.0.0"}`, string(data))
}

func TestReadFileAndRemoveFile(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	require.NoError(t, wt.WriteFile("ns/a.json", []byte("payload")))
	got, err := wt.ReadFile("ns/a.json")
	require.NoError(t, err)
	require.Equal(t, "payload", string(got))

	require.NoError(t, wt.RemoveFile("ns/a.json"))
	_, err = wt.ReadFile("ns/a.json")
	require.Error(t, err)
}

func TestCommitAndPush_CommitsChanges(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	require.NoError(t, wt.WriteFile("feature/app.json", []byte(`{"version":"1.0.0"}`)))
	require.NoError(t, wt.CommitAndPush(ctx, "test: add graph artifact"))

	out := runGit(t, repoDir, "git", "log", "--oneline", branchName, "--", "feature/app.json")
	require.Contains(t, out, "test: add graph artifact")
}

func TestCommitAndPush_NoChanges(t *testing.T) {
	repoDir := initTestRepo(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branchName := "test-graph-" + t.Name()

	wt, err := OpenOrCreate(ctx, branchName)
	require.NoError(t, err)
	defer wt.Remove(ctx)

	require.NoError(t, wt.CommitAndPush(ctx, "test: empty"))
}

func TestCommitAndPush_ReturnsErrorWhenOriginPushFails(t *testing.T) {
	repoDir, _ := initTestRepoWithOrigin(t)
	chdir(t, repoDir)

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
