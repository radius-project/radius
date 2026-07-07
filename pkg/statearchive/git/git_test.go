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
	"time"

	"github.com/radius-project/radius/pkg/statearchive"
	"github.com/stretchr/testify/require"
)

// runGit executes a command (typically "git ...") in dir and asserts success.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "command %v failed: %s", args, out)
	return string(out)
}

// initTestRepo creates a throwaway git repo with one commit on the main branch and returns its
// root. All git operations run relative to a checkout, so tests need a real repo.
func initTestRepo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git-backed test in -short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skipping git-backed test: git binary not found in PATH")
	}

	root := t.TempDir()
	runGit(t, root, "git", "init", "-b", "main")
	runGit(t, root, "git", "config", "user.email", "test@example.com")
	runGit(t, root, "git", "config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(root, "README.md"), []byte("test"), 0o644))
	runGit(t, root, "git", "add", "-A")
	runGit(t, root, "git", "commit", "-m", "initial")
	return root
}

// initTestRepoWithOrigin creates a repo wired to a bare origin remote and returns both paths.
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

// chdir switches to dir for the duration of the test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func TestOpen_CreatesOrphanBranchAndIsolatesState(t *testing.T) {
	root := initTestRepo(t)
	chdir(t, root)

	ctx := context.Background()
	branch := "radius-state-test"
	s, err := NewGitArchive().Open(ctx, branch)
	require.NoError(t, err)
	defer s.Close(ctx)

	// The worktree path is outside the application checkout.
	require.NotEmpty(t, s.Path())
	require.NoDirExists(t, filepath.Join(root, branch))
	require.True(t, branchExists(ctx, root, branch))

	// Writing a state file and committing must not touch the application working tree.
	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	require.NoError(t, s.Commit(ctx, "radius: backup"))

	status := runGit(t, root, "git", "status", "--porcelain")
	require.Empty(t, status, "application working tree must stay clean")
}

func TestOpen_RestoresPreviousState(t *testing.T) {
	root := initTestRepo(t)
	chdir(t, root)

	ctx := context.Background()
	b := NewGitArchive()

	// First session writes state and commits it.
	s1, err := b.Open(ctx, "radius-state-test")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(s1.Path(), "ucp.sql"), []byte("first-dump"), 0o644))
	require.NoError(t, s1.Commit(ctx, "radius: backup 1"))
	s1.Close(ctx)

	// Second session must see the committed state.
	s2, err := b.Open(ctx, "radius-state-test")
	require.NoError(t, err)
	defer s2.Close(ctx)

	data, err := os.ReadFile(filepath.Join(s2.Path(), "ucp.sql"))
	require.NoError(t, err)
	require.Equal(t, "first-dump", string(data))
}

func TestOpen_ReusesBranch(t *testing.T) {
	root := initTestRepo(t)
	chdir(t, root)

	ctx := context.Background()
	b := NewGitArchive()
	branch := "radius-state-test"

	s1, err := b.Open(ctx, branch)
	require.NoError(t, err)
	s1.Close(ctx)

	s2, err := b.Open(ctx, branch)
	require.NoError(t, err)
	defer s2.Close(ctx)

	require.True(t, branchExists(ctx, root, branch))
}

func TestOpen_UsesRemoteBranch(t *testing.T) {
	sourceDir, remoteDir := initTestRepoWithOrigin(t)
	ctx := context.Background()
	branch := "radius-state-test"

	chdir(t, sourceDir)
	seed, err := NewGitArchive().Open(ctx, branch)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(seed.Path(), "app.json"), []byte(`{"version":"seed"}`), 0o644))
	require.NoError(t, seed.Commit(ctx, "test: seed remote artifact"))
	seed.Close(ctx)

	cloneDir := cloneRepo(t, remoteDir)
	chdir(t, cloneDir)

	s, err := NewGitArchive().Open(ctx, branch)
	require.NoError(t, err)
	defer s.Close(ctx)

	data, err := os.ReadFile(filepath.Join(s.Path(), "app.json"))
	require.NoError(t, err)
	require.Equal(t, `{"version":"seed"}`, string(data))
}

func TestCommit_NoRemoteIsNotAnError(t *testing.T) {
	root := initTestRepo(t)
	chdir(t, root)

	ctx := context.Background()
	s, err := NewGitArchive().Open(ctx, "radius-state-test")
	require.NoError(t, err)
	defer s.Close(ctx)

	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	// No origin remote is configured; commit succeeds and the missing remote is tolerated.
	require.NoError(t, s.Commit(ctx, "radius: backup"))
}

func TestCommit_NoChangesIsNoOp(t *testing.T) {
	root := initTestRepo(t)
	chdir(t, root)

	ctx := context.Background()
	s, err := NewGitArchive().Open(ctx, "radius-state-test")
	require.NoError(t, err)
	defer s.Close(ctx)

	require.NoError(t, s.Commit(ctx, "radius: empty"))
}

// TestCommit_NoChangesDoesNotPush verifies that a commit with nothing staged is a true no-op: it
// returns before touching the remote, so a broken remote cannot fail a commit that changed nothing.
func TestCommit_NoChangesDoesNotPush(t *testing.T) {
	repoDir, _ := initTestRepoWithOrigin(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branch := "radius-state-test"
	s, err := NewGitArchive().Open(ctx, branch)
	require.NoError(t, err)
	defer s.Close(ctx)

	// Persist an initial change so the branch exists on the remote.
	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	require.NoError(t, s.Commit(ctx, "radius: backup"))

	// Point origin at a missing remote so any push would fail, then commit with nothing staged.
	runGit(t, repoDir, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))
	require.NoError(t, s.Commit(ctx, "radius: no changes"), "a no-op commit must not attempt a push")
}

func TestCommit_PushesToRemote(t *testing.T) {
	repoDir, _ := initTestRepoWithOrigin(t)
	chdir(t, repoDir)

	ctx := context.Background()
	branch := "radius-state-test"
	s, err := NewGitArchive().Open(ctx, branch)
	require.NoError(t, err)
	defer s.Close(ctx)

	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	require.NoError(t, s.Commit(ctx, "radius: backup"))

	lsRemote := runGit(t, repoDir, "git", "ls-remote", "origin", branch)
	require.Contains(t, lsRemote, branch)
}

func TestCommit_ReturnsErrorWhenPushFails(t *testing.T) {
	repoDir, _ := initTestRepoWithOrigin(t)
	chdir(t, repoDir)

	ctx := context.Background()
	s, err := NewGitArchive().Open(ctx, "radius-state-test")
	require.NoError(t, err)
	defer s.Close(ctx)

	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	runGit(t, repoDir, "git", "remote", "set-url", "origin", filepath.Join(t.TempDir(), "missing.git"))

	err = s.Commit(ctx, "test: push should fail")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to push state branch")
}

// TestCommit_CommitsWithoutConfiguredIdentity verifies the fallback committer identity kicks in
// when the repository has no user.name/user.email, as in a fresh CI runner. Global and system git
// config are isolated so a developer's ambient identity does not mask the fallback.
func TestCommit_CommitsWithoutConfiguredIdentity(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := initTestRepo(t)
	// Remove the identity configured by initTestRepo so no identity is configured anywhere.
	runGit(t, root, "git", "config", "--unset", "user.name")
	runGit(t, root, "git", "config", "--unset", "user.email")
	chdir(t, root)

	ctx := context.Background()
	s, err := NewGitArchive().Open(ctx, "radius-state-test")
	require.NoError(t, err)
	defer s.Close(ctx)

	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	require.NoError(t, s.Commit(ctx, "radius: backup"))

	author := runGit(t, s.Path(), "git", "log", "-1", "--format=%an <%ae>")
	require.Contains(t, author, fallbackUserName)
	require.Contains(t, author, fallbackUserEmail)
}

// TestCommit_CommitsWithOnlyEmailConfigured verifies the fallback identity kicks in when only
// user.email is set but user.name is missing. git needs both to commit, so a partial identity must
// still trigger the fallback rather than failing the commit.
func TestCommit_CommitsWithOnlyEmailConfigured(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := initTestRepo(t)
	// Configure only user.email, leaving user.name unset.
	runGit(t, root, "git", "config", "--unset", "user.name")
	chdir(t, root)

	ctx := context.Background()
	s, err := NewGitArchive().Open(ctx, "radius-state-test")
	require.NoError(t, err)
	defer s.Close(ctx)

	require.NoError(t, os.WriteFile(filepath.Join(s.Path(), "ucp.sql"), []byte("dump"), 0o644))
	require.NoError(t, s.Commit(ctx, "radius: backup"))

	author := runGit(t, s.Path(), "git", "log", "-1", "--format=%an <%ae>")
	require.Contains(t, author, fallbackUserName, "fallback user.name must be used when user.name is unset")
	require.Contains(t, author, fallbackUserEmail)
}

// TestOpen_UnreachableRemoteFailsLoudly verifies that when an origin remote is configured but cannot
// be reached, Open fails loudly instead of treating the remote as "branch absent" and silently
// creating an empty branch (which would restore the wrong, empty state).
func TestOpen_UnreachableRemoteFailsLoudly(t *testing.T) {
	root := initTestRepo(t)
	// Configure an origin that cannot be reached; the branch does not exist locally either.
	runGit(t, root, "git", "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))
	chdir(t, root)

	branch := "radius-state-test"
	_, err := NewGitArchive().Open(context.Background(), branch)
	require.Error(t, err, "Open must fail when the configured remote is unreachable")
	require.False(t, branchExists(context.Background(), root, branch),
		"Open must not create a local branch when it cannot confirm remote state")
}

// TestOpen_OutsideGitRepositoryReturnsError verifies that opening an archive when the working
// directory is not inside a git repository fails with a clear, wrapped error instead of a lower-level
// git message. This is the failure a user hits by running "rad shutdown" / "rad startup" outside a
// repository.
func TestOpen_OutsideGitRepositoryReturnsError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skipping git-backed test: git binary not found in PATH")
	}
	// A bare temp dir with no "git init" is not inside any repository.
	chdir(t, t.TempDir())

	_, err := NewGitArchive().Open(context.Background(), "radius-state-test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to determine git repo root")
}

// TestOpen_SerializesSessionsPerBranch verifies the per-branch lock: a second Open on the same
// branch blocks until the first session is closed (git refuses two worktrees on one branch), while
// Open on a different branch proceeds concurrently. This is the correctness guarantee behind
// branchLocks.
func TestOpen_SerializesSessionsPerBranch(t *testing.T) {
	root := initTestRepo(t)
	chdir(t, root)

	ctx := context.Background()
	b := NewGitArchive()
	const branch = "radius-state-test"

	first, err := b.Open(ctx, branch)
	require.NoError(t, err)

	// A second Open on the SAME branch must block until first is closed.
	sameBranchOpened := make(chan statearchive.Session, 1)
	go func() {
		s, openErr := b.Open(ctx, branch)
		require.NoError(t, openErr)
		sameBranchOpened <- s
	}()

	select {
	case <-sameBranchOpened:
		t.Fatal("second Open on the same branch returned while the first session was still open")
	case <-time.After(300 * time.Millisecond):
		// Still blocked, as expected.
	}

	// A DIFFERENT branch must not be blocked by the held lock.
	other, err := b.Open(ctx, "radius-state-other")
	require.NoError(t, err, "Open on a different branch must not be blocked")
	other.Close(ctx)

	// Closing the first session releases the lock; the queued Open then completes.
	first.Close(ctx)

	select {
	case s := <-sameBranchOpened:
		s.Close(ctx)
	case <-time.After(5 * time.Second):
		t.Fatal("second Open on the same branch did not proceed after the first session was closed")
	}
}
