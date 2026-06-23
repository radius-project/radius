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

// Package gitstate persists Radius state to a git orphan branch.
//
// State files (PostgreSQL dumps and Terraform state) are stored on an orphan branch
// (default "radius-state") that shares no history with the application branches. The branch is
// checked out into a temporary git worktree, completely isolated from the application working
// tree, so state files never appear in the application checkout's "git status".
//
// Lifecycle:
//  1. OpenOrCreate returns a worktree; any files from the previous backup are present in Path.
//  2. Use Path as the state directory for backup/restore operations.
//  3. CommitAndPush commits everything in the worktree and pushes it (when a remote exists).
//  4. Always defer Remove to release the worktree.
package gitstate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultBranch is the default orphan branch name for Radius state.
	DefaultBranch = "radius-state"

	// branchEnvVar overrides the default branch name. It lets parallel tests use isolated
	// branches without colliding.
	branchEnvVar = "RADIUS_STATE_BRANCH"

	// remoteName is the git remote that state is pushed to when it is configured.
	remoteName = "origin"

	// emptyTreeSHA is the well-known SHA of the empty tree, constant across all git repositories.
	// It lets us create an orphan branch without touching the working tree.
	emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"
)

// BranchName returns the state branch name, honoring the RADIUS_STATE_BRANCH environment
// variable and falling back to DefaultBranch.
func BranchName() string {
	if v := os.Getenv(branchEnvVar); v != "" {
		return v
	}
	return DefaultBranch
}

// StateWorktree is a git worktree checked out to the state orphan branch in a temporary
// directory isolated from the application working tree.
type StateWorktree struct {
	// Path is the absolute path of the worktree directory. Use it as the state directory for
	// backup and restore operations.
	Path       string
	branchName string
	repoRoot   string
}

// OpenOrCreate opens a worktree for branchName, creating the orphan branch if it does not yet
// exist. When a remote holds the branch, its latest state is fetched first. Files from a previous
// backup are present in Path when this returns. The caller must defer Remove.
func OpenOrCreate(ctx context.Context, branchName string) (*StateWorktree, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	root, err := repoRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine git repo root: %w", err)
	}

	// If the state branch exists on the remote, fetching it must succeed. Silently falling back
	// to an empty local branch here would make a later restore use stale or empty state, so a
	// fetch failure (network, credentials) is fatal when the remote is known to hold the branch.
	if hasRemote(ctx, root) && remoteHasBranch(ctx, root, branchName) {
		if err := gitExecIn(ctx, root, "fetch", remoteName, branchName); err != nil {
			return nil, fmt.Errorf("failed to fetch state branch %q from %q: %w", branchName, remoteName, err)
		}
	}

	if !branchExists(ctx, root, branchName) {
		if remoteBranchExists(ctx, root, branchName) {
			logger.Info("Creating local state branch from remote", "branch", branchName)
			if err := gitExecIn(ctx, root, "branch", branchName, remoteName+"/"+branchName); err != nil {
				return nil, fmt.Errorf("failed to create local branch %q from remote: %w", branchName, err)
			}
		} else {
			logger.Info("Creating orphan state branch", "branch", branchName)
			if err := createOrphanBranch(ctx, root, branchName); err != nil {
				return nil, fmt.Errorf("failed to create orphan branch %q: %w", branchName, err)
			}
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

// CommitAndPush stages everything in the worktree, commits it, and pushes it to the remote.
//
// The commit is the durable local store. The push is the durable remote store: when a remote is
// configured, a failed push fails the operation (state would otherwise be silently lost). When no
// remote is configured (local development, tests), the commit alone is sufficient and the missing
// remote is not an error.
func (w *StateWorktree) CommitAndPush(ctx context.Context, message string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if err := gitExecIn(ctx, w.Path, "add", "-A"); err != nil {
		return fmt.Errorf("failed to stage state files: %w", err)
	}
	if err := gitExecIn(ctx, w.Path, commitArgs(ctx, w.Path, message)...); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}

	if !hasRemote(ctx, w.repoRoot) {
		logger.Info("No git remote configured; state committed locally only", "branch", w.branchName)
		return nil
	}

	if err := gitExecIn(ctx, w.Path, "push", remoteName, w.branchName); err != nil {
		return fmt.Errorf("failed to push state branch %q to %q: %w", w.branchName, remoteName, err)
	}

	logger.Info("State committed and pushed", "branch", w.branchName)
	return nil
}

// Remove tears down the worktree entry. Always defer this after a successful OpenOrCreate.
func (w *StateWorktree) Remove(ctx context.Context) {
	if err := gitExecIn(ctx, w.repoRoot, "worktree", "remove", "--force", w.Path); err != nil {
		ucplog.FromContextOrDiscard(ctx).Info("Failed to remove git worktree", "path", w.Path, "error", err)
	}
}

// branchExists reports whether branchName exists locally in the repository at root.
func branchExists(ctx context.Context, root, branchName string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "refs/heads/"+branchName)
	cmd.Dir = root
	return cmd.Run() == nil
}

// remoteBranchExists reports whether branchName exists on the remote.
func remoteBranchExists(ctx context.Context, root, branchName string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", remoteName+"/"+branchName)
	cmd.Dir = root
	return cmd.Run() == nil
}

// remoteHasBranch queries the remote directly (without relying on a prior fetch) to report
// whether branchName exists on it. This distinguishes "the branch exists remotely but we could
// not fetch it" (an error) from "the branch does not exist remotely yet" (a normal first run).
func remoteHasBranch(ctx context.Context, root, branchName string) bool {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--exit-code", "--heads", remoteName, branchName)
	cmd.Dir = root
	return cmd.Run() == nil
}

// commitArgs builds the git arguments for committing the worktree, injecting a fallback identity
// when the repository has no user.name/user.email configured. Fresh CI environments frequently
// lack a git identity, which would otherwise make the commit (and therefore rad shutdown) fail
// even though the backup itself succeeded.
func commitArgs(ctx context.Context, dir, message string) []string {
	var args []string
	if !gitIdentityConfigured(ctx, dir) {
		args = append(args, "-c", "user.name=Radius", "-c", "user.email=radius@radapp.io")
	}
	return append(args, "commit", "-m", message, "--allow-empty")
}

// gitIdentityConfigured reports whether user.email is set for the repository at dir.
func gitIdentityConfigured(ctx context.Context, dir string) bool {
	cmd := exec.CommandContext(ctx, "git", "config", "user.email")
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

// createOrphanBranch creates an empty orphan branch using low-level git plumbing without touching
// the working tree or switching branches.
func createOrphanBranch(ctx context.Context, root, branchName string) error {
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
