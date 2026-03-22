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

// ---------------------------------------------------------------------------
// OpenOrCreate — happy paths
// ---------------------------------------------------------------------------

func Test_OpenOrCreate_FirstRun(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	require.DirExists(t, w.Path)
	require.Equal(t, SemaphoreFirstRun, w.CheckSemaphore())
}

func Test_OpenOrCreate_ExistingBranch(t *testing.T) {
	initGitRepo(t)

	// First open creates the branch.
	w1, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	w1.Remove(context.Background())

	// Second open reuses the existing branch.
	w2, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w2.Remove(context.Background())

	require.DirExists(t, w2.Path)
}

// ---------------------------------------------------------------------------
// OpenOrCreate — error paths
// ---------------------------------------------------------------------------

func Test_OpenOrCreate_NotInGitRepo(t *testing.T) {
	// A plain temp dir with no .git → repoRoot returns an error.
	dir := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(dir))

	_, err = OpenOrCreate(context.Background(), DefaultBranch)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to determine git repo root")
}

func Test_OpenOrCreate_ObjectsDirReadOnly_OrphanBranchFails(t *testing.T) {
	// Making .git/objects non-writable prevents commit-tree from writing a new
	// commit object, exercising the createOrphanBranch error path.
	dir := initGitRepo(t)

	objectsDir := filepath.Join(dir, ".git", "objects")
	require.NoError(t, os.Chmod(objectsDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(objectsDir, 0o755) })

	_, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to create orphan branch")
}

func Test_OpenOrCreate_BranchAlreadyCheckedOut_WorktreeAddFails(t *testing.T) {
	initGitRepo(t)

	// First open: keep the worktree alive so the branch stays checked out.
	w1, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w1.Remove(context.Background())

	// Second open of the same branch fails because git only allows a branch to
	// be checked out in one worktree at a time.
	_, err = OpenOrCreate(context.Background(), DefaultBranch)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to add worktree")
}

// ---------------------------------------------------------------------------
// Semaphore — happy paths
// ---------------------------------------------------------------------------

func Test_Semaphore_WriteLock_SetsInterrupted(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, SemaphoreFirstRun, w.CheckSemaphore())
	require.NoError(t, w.WriteLock(context.Background()))
	w.Remove(context.Background())

	w2, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w2.Remove(context.Background())

	require.Equal(t, SemaphoreInterrupted, w2.CheckSemaphore())
}

func Test_Semaphore_ClearLock_SetsClean(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)

	require.NoError(t, w.WriteLock(context.Background()))
	require.NoError(t, os.WriteFile(filepath.Join(w.Path, "ucp.sql"), []byte("-- sql"), 0o644))
	require.NoError(t, w.ClearLock(context.Background()))
	w.Remove(context.Background())

	w2, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w2.Remove(context.Background())

	require.Equal(t, SemaphoreClean, w2.CheckSemaphore())

	_, err = os.Stat(filepath.Join(w2.Path, "ucp.sql"))
	require.NoError(t, err, "backup file must survive ClearLock round-trip")
}

func Test_Semaphore_WriteLock_RemovesBackupOK(t *testing.T) {
	initGitRepo(t)

	// Reach a clean state first.
	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(w.Path, "ucp.sql"), []byte("-- sql"), 0o644))
	require.NoError(t, w.ClearLock(context.Background()))
	w.Remove(context.Background())

	// New run: open → clean → write lock → branch looks interrupted.
	w2, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, SemaphoreClean, w2.CheckSemaphore())
	require.NoError(t, w2.WriteLock(context.Background()))
	w2.Remove(context.Background())

	w3, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w3.Remove(context.Background())
	require.Equal(t, SemaphoreInterrupted, w3.CheckSemaphore())
}

// ---------------------------------------------------------------------------
// WriteLock / ClearLock — error paths (read-only worktree directory)
// ---------------------------------------------------------------------------

func Test_WriteLock_ReadOnlyDir_Error(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer func() {
		_ = os.Chmod(w.Path, 0o755)
		w.Remove(context.Background())
	}()

	require.NoError(t, os.Chmod(w.Path, 0o555))

	err = w.WriteLock(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to write lock file")
}

func Test_ClearLock_ReadOnlyDir_Error(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer func() {
		_ = os.Chmod(w.Path, 0o755)
		w.Remove(context.Background())
	}()

	require.NoError(t, os.Chmod(w.Path, 0o555))

	err = w.ClearLock(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to write backup-ok file")
}

// ---------------------------------------------------------------------------
// commit — git-add failure (corrupted worktree .git file)
// ---------------------------------------------------------------------------

func Test_WriteLock_CorruptedWorktree_CommitFails(t *testing.T) {
	// In a git linked worktree the .git entry is a regular file pointing to the
	// main repo's worktree metadata. Deleting it makes the directory unrecognised
	// by git, so git add -A fails. This exercises the commit() error path.
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	require.NoError(t, os.Remove(filepath.Join(w.Path, ".git")))

	err = w.WriteLock(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to stage state files")
}

func Test_ClearLock_CorruptedWorktree_CommitFails(t *testing.T) {
	// ClearLock writes .backup-ok first (succeeds), then calls commit which
	// needs git. Deleting the .git file after a successful WriteLock causes
	// ClearLock's commit call to fail, covering its error return path.
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	// Get to a WriteLock-committed state so .git is intact.
	require.NoError(t, w.WriteLock(context.Background()))

	// Now corrupt the worktree so git commands fail.
	require.NoError(t, os.Remove(filepath.Join(w.Path, ".git")))

	// .backup-ok WriteFile succeeds (path exists), but the subsequent commit fails.
	err = w.ClearLock(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to stage state files")
}

func Test_Commit_GitAddSucceeds_GitCommitFails(t *testing.T) {
	// To hit the "failed to commit state" error branch, we need git add to
	// succeed but git commit to fail. We achieve this by making .git/objects
	// non-writable *after* git add has written to the index — git commit then
	// cannot create the commit object.
	dir := initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer func() {
		// Restore write access for Remove to work.
		_ = os.Chmod(filepath.Join(dir, ".git", "objects"), 0o755)
		w.Remove(context.Background())
	}()

	// Stage a file manually so the index is dirty.
	require.NoError(t, os.WriteFile(filepath.Join(w.Path, "staged.txt"), []byte("x"), 0o644))
	// Run git add successfully.
	require.NoError(t, gitExecIn(context.Background(), w.Path, "add", "-A"))

	// Now block new object writes so git commit cannot create the commit object.
	objectsDir := filepath.Join(dir, ".git", "objects")
	require.NoError(t, os.Chmod(objectsDir, 0o555))

	// Call commit directly (not through WriteLock/ClearLock to isolate the branch).
	err = w.commit(context.Background(), "should fail")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to commit state")
}

// ---------------------------------------------------------------------------
// Remove — error path (worktree already removed)
// ---------------------------------------------------------------------------

func Test_Remove_AlreadyRemoved_DoesNotPanic(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)

	// First Remove tears down the worktree dir and unregisters it.
	w.Remove(context.Background())

	// Second Remove: git returns an error (worktree not known); Remove just logs it.
	// The call must not panic.
	w.Remove(context.Background())
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

func Test_DefaultBranch_Constant(t *testing.T) {
	require.Equal(t, "radius-state", DefaultBranch)
}

// ---------------------------------------------------------------------------
// NewLockInfoFromEnv
// ---------------------------------------------------------------------------

func Test_NewLockInfoFromEnv_DefaultsWhenUnset(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "")
	t.Setenv("GITHUB_RUN_ATTEMPT", "")
	t.Setenv("GITHUB_JOB", "")
	t.Setenv("GITHUB_REPOSITORY", "")

	info := NewLockInfoFromEnv()

	require.Equal(t, "local", info.RunID)
	require.Equal(t, 1, info.RunAttempt)
	require.Equal(t, "", info.JobName)
	require.Equal(t, "", info.Repository)
	require.False(t, info.AcquiredAt.IsZero())
}

func Test_NewLockInfoFromEnv_ReadsEnvVars(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "99887766")
	t.Setenv("GITHUB_RUN_ATTEMPT", "3")
	t.Setenv("GITHUB_JOB", "deploy")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")

	info := NewLockInfoFromEnv()

	require.Equal(t, "99887766", info.RunID)
	require.Equal(t, 3, info.RunAttempt)
	require.Equal(t, "deploy", info.JobName)
	require.Equal(t, "org/repo", info.Repository)
}

func Test_NewLockInfoFromEnv_InvalidAttemptDefaultsToOne(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "123")
	t.Setenv("GITHUB_RUN_ATTEMPT", "notanumber")

	info := NewLockInfoFromEnv()

	require.Equal(t, 1, info.RunAttempt)
}

// ---------------------------------------------------------------------------
// ErrDeployLockHeld
// ---------------------------------------------------------------------------

func Test_ErrDeployLockHeld_ErrorMessage(t *testing.T) {
	err := ErrDeployLockHeld{Existing: LockInfo{
		RunID:      "42",
		RunAttempt: 1,
		JobName:    "deploy-job",
	}}
	require.Contains(t, err.Error(), "run 42 attempt 1")
	require.Contains(t, err.Error(), "deploy-job")
}

// ---------------------------------------------------------------------------
// TryAcquireDeployLock / ReleaseDeployLock
// ---------------------------------------------------------------------------

func Test_TryAcquireDeployLock_FirstAcquire(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	info := LockInfo{RunID: "run1", RunAttempt: 1, JobName: "build"}
	err = w.TryAcquireDeployLock(context.Background(), info)
	require.NoError(t, err)

	// Lock file must be present in the worktree.
	_, statErr := os.Stat(filepath.Join(w.Path, deployLockFile))
	require.NoError(t, statErr)
}

func Test_TryAcquireDeployLock_TakeoverStaleLock(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	// First attempt acquires the lock.
	attempt1 := LockInfo{RunID: "runX", RunAttempt: 1, JobName: "build"}
	require.NoError(t, w.TryAcquireDeployLock(context.Background(), attempt1))

	// Second attempt of the SAME run can take over the stale lock.
	attempt2 := LockInfo{RunID: "runX", RunAttempt: 2, JobName: "build"}
	err = w.TryAcquireDeployLock(context.Background(), attempt2)
	require.NoError(t, err, "retry should take over stale lock from earlier attempt")
}

func Test_TryAcquireDeployLock_HeldBySameAttempt(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	info := LockInfo{RunID: "runA", RunAttempt: 1}
	require.NoError(t, w.TryAcquireDeployLock(context.Background(), info))

	// Same run, same attempt → lock is held.
	err = w.TryAcquireDeployLock(context.Background(), info)
	var held ErrDeployLockHeld
	require.ErrorAs(t, err, &held)
	require.Equal(t, "runA", held.Existing.RunID)
}

func Test_TryAcquireDeployLock_HeldByDifferentRun(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	// Run A acquires.
	require.NoError(t, w.TryAcquireDeployLock(context.Background(), LockInfo{RunID: "runA", RunAttempt: 1}))

	// Run B cannot take over because the run IDs differ.
	err = w.TryAcquireDeployLock(context.Background(), LockInfo{RunID: "runB", RunAttempt: 1})
	var held ErrDeployLockHeld
	require.ErrorAs(t, err, &held)
	require.Equal(t, "runA", held.Existing.RunID)
}

func Test_TryAcquireDeployLock_CorruptLockFile(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	// Write a corrupt (non-JSON) lock file directly.
	require.NoError(t, os.WriteFile(filepath.Join(w.Path, deployLockFile), []byte("not-json"), 0o644))

	err = w.TryAcquireDeployLock(context.Background(), LockInfo{RunID: "run1", RunAttempt: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "corrupt")
}

func Test_TryAcquireDeployLock_ReadOnlyDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; read-only dirs are not enforced")
	}

	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chmod(w.Path, 0o755)
		w.Remove(context.Background())
	})

	require.NoError(t, os.Chmod(w.Path, 0o555))

	err = w.TryAcquireDeployLock(context.Background(), LockInfo{RunID: "r", RunAttempt: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to write deploy lock")
}

func Test_ReleaseDeployLock_RemovesFile(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	require.NoError(t, w.TryAcquireDeployLock(context.Background(), LockInfo{RunID: "r1", RunAttempt: 1}))

	// File must be present after acquire.
	_, statErr := os.Stat(filepath.Join(w.Path, deployLockFile))
	require.NoError(t, statErr)

	require.NoError(t, w.ReleaseDeployLock(context.Background()))

	// File must be gone after release.
	_, statErr = os.Stat(filepath.Join(w.Path, deployLockFile))
	require.True(t, os.IsNotExist(statErr))
}

func Test_ReleaseDeployLock_AlreadyAbsent_Succeeds(t *testing.T) {
	initGitRepo(t)

	w, err := OpenOrCreate(context.Background(), DefaultBranch)
	require.NoError(t, err)
	defer w.Remove(context.Background())

	// Release without a prior acquire — should succeed (file removal is best-effort).
	require.NoError(t, w.ReleaseDeployLock(context.Background()))
}
