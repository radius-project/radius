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

// Package git implements the statearchive.Archive interface on top of a git
// orphan branch.
//
// An archive name maps to an orphan branch that shares no history with the
// application branches. Opening a session checks that branch out into a
// temporary git worktree, isolated from the application working tree, so state
// files never appear in the application checkout's "git status". Committing a
// session commits every change in the worktree and pushes it to the remote
// when one is configured.
//
// This is the single, shared implementation of the "orphan branch as durable
// store" primitive that previously lived, duplicated, in both the app graph
// store and the CLI state code.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/radius-project/radius/pkg/statearchive"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// remoteName is the git remote that state is pushed to when it is configured.
	remoteName = "origin"

	// emptyTreeSHA is the well-known SHA of the empty tree, constant across all git
	// repositories. It lets us create an orphan branch without touching the working tree.
	emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

	// fallbackUserName and fallbackUserEmail are the committer identity used when the
	// repository has no user.name/user.email configured. Fresh CI environments frequently lack a
	// git identity, which would otherwise make the commit fail even though the backup succeeded.
	fallbackUserName  = "Radius"
	fallbackUserEmail = "radius@radapp.io"
)

// branchLocks serializes sessions per branch. git worktree add refuses to add a second worktree
// for a branch that is already checked out, so two concurrent sessions on the same branch would
// otherwise fail nondeterministically. The lock is held from Open until Close.
var branchLocks sync.Map // branch name -> *sync.Mutex

func lockForBranch(branch string) *sync.Mutex {
	v, _ := branchLocks.LoadOrStore(branch, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// GitArchive is a statearchive.Archive backed by git orphan branches. The repository is
// auto-detected via "git rev-parse --show-toplevel" at Open time, so no path is required up front.
type GitArchive struct{}

// NewGitArchive returns a git-backed statearchive.Archive.
func NewGitArchive() *GitArchive {
	return &GitArchive{}
}

// Open checks the orphan branch named branch out into a temporary worktree, creating the branch
// (from the remote, or empty) if it does not yet exist. The returned Session holds a per-branch
// lock until Close.
func (b *GitArchive) Open(ctx context.Context, branch string) (statearchive.Session, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	lock := lockForBranch(branch)
	lock.Lock()

	// Any failure before we return the session must release the lock, otherwise the branch would
	// be permanently blocked.
	unlockOnErr := true
	defer func() {
		if unlockOnErr {
			lock.Unlock()
		}
	}()

	root, err := repoRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine git repo root: %w", err)
	}

	// If the branch exists on the remote, fetching it must succeed. Silently falling back to an
	// empty or stale local branch would make a later restore use the wrong state, so a fetch
	// failure (network, credentials) is fatal when the remote is known to hold the branch.
	if hasRemote(ctx, root) && remoteHasBranch(ctx, root, branch) {
		logger.Info("Fetching remote state branch", "branch", branch)
		if err := gitExecIn(ctx, root, "fetch", remoteName, branch); err != nil {
			return nil, fmt.Errorf("failed to fetch state branch %q from %q: %w", branch, remoteName, err)
		}
		// Force the local branch to match the remote so a stale local branch cannot shadow it.
		if err := gitExecIn(ctx, root, "branch", "--force", branch, remoteName+"/"+branch); err != nil {
			return nil, fmt.Errorf("failed to sync local branch %q to remote: %w", branch, err)
		}
	}

	if !branchExists(ctx, root, branch) {
		logger.Info("Creating orphan state branch", "branch", branch)
		if err := createOrphanBranch(ctx, root, branch); err != nil {
			return nil, fmt.Errorf("failed to create orphan branch %q: %w", branch, err)
		}
	}

	// git worktree add requires the target path to not exist; git creates it.
	wtPath := filepath.Join(os.TempDir(), fmt.Sprintf("radius-state-%d", time.Now().UnixNano()))
	logger.Info("Adding git worktree", "path", wtPath, "branch", branch)
	if err := gitExecIn(ctx, root, "worktree", "add", wtPath, branch); err != nil {
		return nil, fmt.Errorf("failed to add worktree: %w", err)
	}

	unlockOnErr = false
	return &session{
		path:     wtPath,
		branch:   branch,
		repoRoot: root,
		unlock:   lock.Unlock,
	}, nil
}

// session is a storage.Session backed by a git worktree checked out to an orphan branch.
type session struct {
	path     string
	branch   string
	repoRoot string

	unlock   func()
	unlocked bool
}

// Path returns the absolute path of the worktree directory.
func (s *session) Path() string { return s.path }

// Commit stages every change in the worktree, commits it, and pushes it to the remote.
//
// The commit is the durable local store. The push is the durable remote store: when a remote is
// configured, a failed push fails the operation (state would otherwise be silently lost). When no
// remote is configured (local development, tests), the commit alone is sufficient and the missing
// remote is not an error. Committing with no staged changes is a no-op.
func (s *session) Commit(ctx context.Context, message string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if err := gitExecIn(ctx, s.path, "add", "-A"); err != nil {
		return fmt.Errorf("failed to stage state files: %w", err)
	}

	// Nothing staged means nothing to persist.
	if gitExecIn(ctx, s.path, "diff", "--cached", "--quiet") == nil {
		logger.Info("No state changes to commit", "branch", s.branch)
	} else if err := gitExecIn(ctx, s.path, s.commitArgs(ctx, message)...); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}

	if !hasRemote(ctx, s.repoRoot) {
		logger.Info("No git remote configured; state committed locally only", "branch", s.branch)
		return nil
	}

	if err := gitExecIn(ctx, s.path, "push", remoteName, s.branch); err != nil {
		return fmt.Errorf("failed to push state branch %q to %q: %w", s.branch, remoteName, err)
	}

	logger.Info("State committed and pushed", "branch", s.branch)
	return nil
}

// Close removes the worktree entry and releases the per-branch lock. Always defer it after a
// successful Open.
func (s *session) Close(ctx context.Context) {
	if err := gitExecIn(ctx, s.repoRoot, "worktree", "remove", "--force", s.path); err != nil {
		ucplog.FromContextOrDiscard(ctx).Info("Failed to remove git worktree", "path", s.path, "error", err)
	}
	if !s.unlocked {
		s.unlocked = true
		s.unlock()
	}
}

// commitArgs builds the git arguments for committing the worktree, injecting a fallback identity
// via "-c" flags (rather than mutating repo config) when the repository has no user identity.
func (s *session) commitArgs(ctx context.Context, message string) []string {
	var args []string
	if !gitIdentityConfigured(ctx, s.path) {
		args = append(args, "-c", "user.name="+fallbackUserName, "-c", "user.email="+fallbackUserEmail)
	}
	return append(args, "commit", "-m", message)
}

// gitIdentityConfigured reports whether user.email is set for the repository at dir.
func gitIdentityConfigured(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", "user.email")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// hasRemote reports whether a remote named origin is configured.
func hasRemote(ctx context.Context, root string) bool {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", remoteName)
	cmd.Dir = root
	return cmd.Run() == nil
}

// remoteHasBranch queries the remote directly (without relying on a prior fetch) to report whether
// branch exists on it. This distinguishes "the branch exists remotely but we could not fetch it"
// (an error) from "the branch does not exist remotely yet" (a normal first run).
func remoteHasBranch(ctx context.Context, root, branch string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", "--heads", remoteName, branch)
	cmd.Dir = root
	return cmd.Run() == nil
}

// branchExists reports whether branch exists locally in the repository at root.
func branchExists(ctx context.Context, root, branch string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "refs/heads/"+branch)
	cmd.Dir = root
	return cmd.Run() == nil
}

// createOrphanBranch creates an empty orphan branch using low-level git plumbing without touching
// the working tree or switching branches. It relies on the well-known empty-tree SHA that is
// constant across all git repositories.
func createOrphanBranch(ctx context.Context, root, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "commit-tree", emptyTreeSHA, "-m", "radius: init state branch")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit-tree: %w: %s", err, stderr.String())
	}

	commitSHA := strings.TrimSpace(stdout.String())
	return gitExecIn(ctx, root, "update-ref", "refs/heads/"+branch, commitSHA)
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

// Compile-time checks that the git implementation satisfies the statearchive interfaces.
var (
	_ statearchive.Archive = (*GitArchive)(nil)
	_ statearchive.Session = (*session)(nil)
)
