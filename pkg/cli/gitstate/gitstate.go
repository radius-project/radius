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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultBranch is the default orphan branch name for Radius state.
	DefaultBranch = "radius-state"

	lockFile       = ".lock"
	backupOKFile   = ".backup-ok"
	deployLockFile = ".deploy-lock"
)

// SemaphoreState describes the condition of the state branch detected from sentinel files.
type SemaphoreState int

const (
	// SemaphoreFirstRun indicates no prior backup state exists (branch is fresh).
	SemaphoreFirstRun SemaphoreState = iota

	// SemaphoreInterrupted indicates a previous run wrote .lock but never cleared it,
	// suggesting the runner was evicted mid-deploy (e.g. a spot instance).
	SemaphoreInterrupted

	// SemaphoreClean indicates the previous run shut down cleanly (.backup-ok present, no .lock).
	SemaphoreClean
)

// LockInfo records the identity of the GitHub Actions run that holds the deploy lock.
// It is serialised as JSON into the .deploy-lock file in the state worktree.
type LockInfo struct {
	// RunID is the value of GITHUB_RUN_ID for the acquiring run.
	RunID string `json:"runID"`
	// RunAttempt is the value of GITHUB_RUN_ATTEMPT for the acquiring run (1-based).
	// A retry increments this; TryAcquireDeployLock uses it to detect stale locks.
	RunAttempt int `json:"runAttempt"`
	// JobName is the value of GITHUB_JOB for the acquiring run.
	JobName string `json:"jobName"`
	// Repository is the value of GITHUB_REPOSITORY for the acquiring run.
	Repository string `json:"repository"`
	// AcquiredAt is the UTC time when the lock was written.
	AcquiredAt time.Time `json:"acquiredAt"`
}

// ErrDeployLockHeld is returned by TryAcquireDeployLock when the lock is held by a run
// that is not superseded by the current attempt.
type ErrDeployLockHeld struct {
	Existing LockInfo
}

func (e ErrDeployLockHeld) Error() string {
	return fmt.Sprintf(
		"deploy is already in progress: run %s attempt %d (job %q, acquired %s). "+
			"If that run has ended, re-run this workflow to retry.",
		e.Existing.RunID, e.Existing.RunAttempt,
		e.Existing.JobName, e.Existing.AcquiredAt.Format(time.RFC3339),
	)
}

// NewLockInfoFromEnv builds a LockInfo from the standard GitHub Actions environment variables.
// When those variables are not set (local development), sensible fallback values are used.
func NewLockInfoFromEnv() LockInfo {
	attempt := 1
	if s := os.Getenv("GITHUB_RUN_ATTEMPT"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			attempt = n
		}
	}
	return LockInfo{
		RunID:      envOrDefault("GITHUB_RUN_ID", "local"),
		RunAttempt: attempt,
		JobName:    os.Getenv("GITHUB_JOB"),
		Repository: os.Getenv("GITHUB_REPOSITORY"),
		AcquiredAt: time.Now().UTC(),
	}
}

// envOrDefault returns the value of the named environment variable, or def if it is unset.
func envOrDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// StateWorktree is a git worktree checked out to the state orphan branch in a temporary
// directory that is completely isolated from the application working tree. State files
// (SQL backups, sentinel files) live only in the orphan branch — never in the app checkout.
//
// Lifecycle:
//  1. Call OpenOrCreate to get a worktree. Files from the previous backup are present in Path.
//  2. Call CheckSemaphore to determine whether prior state can be trusted.
//  3. Call WriteLock before starting cluster creation (spot-instance safety).
//  4. Use Path as the stateDir for pgbackup operations.
//  5. Call ClearLock after a successful backup; this commits and pushes everything.
//  6. Always defer Remove to release the worktree.
type StateWorktree struct {
	// Path is the absolute path of the worktree directory.
	// Use this as the stateDir argument for pgbackup operations.
	Path       string
	branchName string
	repoRoot   string
}

// OpenOrCreate opens a git worktree for branchName, creating the orphan branch if it does
// not yet exist, and returns the worktree. The worktree is placed in a system temp directory.
// Files from a previous backup are already present in Path after this call returns.
//
// The caller must always defer Remove to avoid stale worktree entries.
func OpenOrCreate(ctx context.Context, branchName string) (*StateWorktree, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	root, err := repoRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine git repo root: %w", err)
	}

	if !branchExists(ctx, root, branchName) {
		logger.Info("Creating orphan state branch", "branch", branchName)
		if err := createOrphanBranch(ctx, root, branchName); err != nil {
			return nil, fmt.Errorf("failed to create orphan branch %q: %w", branchName, err)
		}
	}

	// git worktree add requires the target path to not exist; git creates it.
	wtPath := filepath.Join(os.TempDir(), fmt.Sprintf("radius-state-%d", time.Now().UnixNano()))
	logger.Info("Adding git worktree", "path", wtPath, "branch", branchName)
	if err := gitExecIn(ctx, root, "worktree", "add", wtPath, branchName); err != nil {
		return nil, fmt.Errorf("failed to add worktree: %w", err)
	}

	return &StateWorktree{
		Path:       wtPath,
		branchName: branchName,
		repoRoot:   root,
	}, nil
}

// CheckSemaphore reads the sentinel files in the worktree to determine the outcome of the
// previous run. Must be called before WriteLock so the pre-lock state is captured.
func (w *StateWorktree) CheckSemaphore() SemaphoreState {
	_, lockErr := os.Stat(filepath.Join(w.Path, lockFile))
	_, okErr := os.Stat(filepath.Join(w.Path, backupOKFile))

	switch {
	case lockErr == nil:
		return SemaphoreInterrupted
	case okErr == nil:
		return SemaphoreClean
	default:
		return SemaphoreFirstRun
	}
}

// WriteLock writes the .lock sentinel, commits, and pushes to mark a run as in-progress.
// Call this before creating the cluster so any spot-instance eviction is detectable on the
// next run. Capture the semaphore state with CheckSemaphore before calling this.
func (w *StateWorktree) WriteLock(ctx context.Context) error {
	if err := os.WriteFile(filepath.Join(w.Path, lockFile), []byte(""), 0o644); err != nil {
		return fmt.Errorf("failed to write lock file: %w", err)
	}
	// Remove any leftover .backup-ok from a previous clean run.
	_ = os.Remove(filepath.Join(w.Path, backupOKFile))

	if err := w.commit(ctx, "radius: begin deploy (lock)"); err != nil {
		return err
	}
	return w.Push(ctx)
}

// ClearLock writes .backup-ok, removes .lock, stages all files in the worktree
// (including SQL backup files written by pgbackup), commits, and pushes.
// This is the final step of a successful rad shutdown.
func (w *StateWorktree) ClearLock(ctx context.Context) error {
	if err := os.WriteFile(filepath.Join(w.Path, backupOKFile), []byte(""), 0o644); err != nil {
		return fmt.Errorf("failed to write backup-ok file: %w", err)
	}
	_ = os.Remove(filepath.Join(w.Path, lockFile))

	if err := w.commit(ctx, "radius: shutdown complete (backup-ok)"); err != nil {
		return err
	}
	return w.Push(ctx)
}

// TryAcquireDeployLock attempts to write a .deploy-lock file to the worktree, recording
// the supplied LockInfo. If a lock already exists and belongs to the same workflow run at
// a lower attempt number (i.e., a previously failed retry), the stale lock is silently
// taken over. In all other cases where a lock already exists, ErrDeployLockHeld is returned.
//
// Call this at the start of rad deploy. Always pair with a deferred ReleaseDeployLock.
func (w *StateWorktree) TryAcquireDeployLock(ctx context.Context, info LockInfo) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	lockPath := filepath.Join(w.Path, deployLockFile)

	if data, err := os.ReadFile(lockPath); err == nil {
		var existing LockInfo
		if jsonErr := json.Unmarshal(data, &existing); jsonErr != nil {
			return fmt.Errorf("deploy lock file is corrupt: %w", jsonErr)
		}
		if existing.RunID == info.RunID && existing.RunAttempt < info.RunAttempt {
			// Same workflow run, earlier attempt failed — safe to take over.
			logger.Info("Taking over stale deploy lock from previous attempt",
				"previousAttempt", existing.RunAttempt,
				"currentAttempt", info.RunAttempt)
			// fall through to overwrite
		} else {
			return ErrDeployLockHeld{Existing: existing}
		}
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal deploy lock info: %w", err)
	}
	if err := os.WriteFile(lockPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write deploy lock: %w", err)
	}
	if err := w.commit(ctx, fmt.Sprintf("radius: acquire deploy lock (run %s attempt %d)", info.RunID, info.RunAttempt)); err != nil {
		return err
	}
	return w.Push(ctx)
}

// ReleaseDeployLock removes the .deploy-lock file, commits, and pushes. It is non-fatal if
// the lock file is already absent (e.g. the worktree was cleaned up by another means).
// Always call this (via defer) after a successful TryAcquireDeployLock.
func (w *StateWorktree) ReleaseDeployLock(ctx context.Context) error {
	_ = os.Remove(filepath.Join(w.Path, deployLockFile))
	if err := w.commit(ctx, "radius: release deploy lock"); err != nil {
		return err
	}
	return w.Push(ctx)
}

// Push pushes the state branch to origin. A missing remote is treated as a non-fatal
// warning so that local development and unit tests work without a configured remote.
func (w *StateWorktree) Push(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	if err := gitExecIn(ctx, w.Path, "push", "origin", w.branchName); err != nil {
		logger.Info("Could not push state branch (no remote configured?)",
			"branch", w.branchName, "error", err)
	}
	return nil // non-fatal: spot-instance safety degrades gracefully in local dev
}

// Remove tears down the worktree entry. Always defer this after a successful OpenOrCreate.
func (w *StateWorktree) Remove(ctx context.Context) {
	if err := gitExecIn(ctx, w.repoRoot, "worktree", "remove", "--force", w.Path); err != nil {
		ucplog.FromContextOrDiscard(ctx).Info("Failed to remove git worktree",
			"path", w.Path, "error", err)
	}
}

// commit stages all changes in the worktree and creates a commit.
func (w *StateWorktree) commit(ctx context.Context, msg string) error {
	if err := gitExecIn(ctx, w.Path, "add", "-A"); err != nil {
		return fmt.Errorf("failed to stage state files: %w", err)
	}
	if err := gitExecIn(ctx, w.Path, "commit", "-m", msg, "--allow-empty"); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}
	return nil
}

// branchExists reports whether branchName exists in the repository at root.
func branchExists(ctx context.Context, root, branchName string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", branchName)
	cmd.Dir = root
	return cmd.Run() == nil
}

// createOrphanBranch creates an empty orphan branch using low-level git plumbing
// without touching the current working tree or switching branches.
// It relies on the well-known empty-tree SHA that is constant across all git repositories.
func createOrphanBranch(ctx context.Context, root, branchName string) error {
	// 4b825dc642cb6eb9a060e54bf8d69288fbee4904 is the SHA of the empty tree in git.
	const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

	cmd := exec.CommandContext(ctx, "git", "commit-tree", emptyTreeSHA, "-m", "radius: init state branch")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit-tree: %w: %s", err, stderr.String())
	}

	commitSHA := strings.TrimSpace(stdout.String())
	return gitExecIn(ctx, root, "update-ref", "refs/heads/"+branchName, commitSHA)
}

// repoRoot returns the absolute path to the root of the current git repository.
func repoRoot(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// gitExecIn runs a git command with its working directory set to dir.
func gitExecIn(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}
