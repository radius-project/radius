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

// Package gitstate manages state stored on orphan branches in a git repository.
// It uses git worktrees so that files on the orphan branch never pollute the
// application working tree. This is adapted from the filesystem-state branch
// implementation for storing Radius state.
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
	// DefaultGraphBranch is the default orphan branch name for graph artifacts.
	DefaultGraphBranch = "radius-graph"
)

// StateWorktree is a git worktree checked out to an orphan branch in a temporary
// directory that is completely isolated from the application working tree.
//
// Lifecycle:
//  1. Call OpenOrCreate to get a worktree.
//  2. Write files into Path (using WriteFile or directly).
//  3. Call CommitAndPush to commit and push all changes.
//  4. Always defer Remove to release the worktree.
type StateWorktree struct {
	// Path is the absolute path of the worktree directory.
	Path       string
	branchName string
	repoRoot   string
}

// OpenOrCreate opens a git worktree for branchName, creating the orphan branch if it does
// not yet exist, and returns the worktree. The worktree is placed in a system temp directory.
//
// The caller must always defer Remove to avoid stale worktree entries.
func OpenOrCreate(ctx context.Context, branchName string) (*StateWorktree, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	root, err := repoRoot(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to determine git repo root: %w", err)
	}

	hasOrigin, err := originRemoteExists(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("checking origin remote: %w", err)
	}

	if hasOrigin {
		remoteExists, err := remoteBranchExists(ctx, root, branchName)
		if err != nil {
			return nil, fmt.Errorf("checking remote branch %q: %w", branchName, err)
		}

		if remoteExists {
			logger.Info("Fetching remote state branch", "branch", branchName)
			if err := fetchRemoteBranch(ctx, root, branchName); err != nil {
				return nil, fmt.Errorf("fetching remote branch %q: %w", branchName, err)
			}

			if err := syncLocalBranchToRemote(ctx, root, branchName); err != nil {
				return nil, fmt.Errorf("syncing local branch %q to remote: %w", branchName, err)
			}
		}
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

// WriteFile writes data to the given relative path inside the worktree,
// creating intermediate directories as needed.
func (w *StateWorktree) WriteFile(relPath string, data []byte) error {
	fullPath := filepath.Join(w.Path, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return fmt.Errorf("creating directories for %s: %w", relPath, err)
	}
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", relPath, err)
	}
	return nil
}

// CommitAndPush stages all changes in the worktree, commits, and pushes.
func (w *StateWorktree) CommitAndPush(ctx context.Context, msg string) error {
	if err := w.commit(ctx, msg); err != nil {
		return err
	}
	return w.push(ctx)
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
	// Configure committer identity for CI environments.
	_ = gitExecIn(ctx, w.Path, "config", "user.name", "github-actions[bot]")
	_ = gitExecIn(ctx, w.Path, "config", "user.email", "github-actions[bot]@users.noreply.github.com")

	if err := gitExecIn(ctx, w.Path, "add", "-A"); err != nil {
		return fmt.Errorf("failed to stage state files: %w", err)
	}

	// Check if there are changes to commit.
	if err := gitExecIn(ctx, w.Path, "diff", "--cached", "--quiet"); err == nil {
		// No changes to commit.
		return nil
	}

	if err := gitExecIn(ctx, w.Path, "commit", "-m", msg); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}
	return nil
}

// push pushes the state branch to origin. A missing remote is treated as a non-fatal
// condition for local development and unit tests, but any push failure with an
// existing origin is returned so CI cannot report success for a stale artifact.
func (w *StateWorktree) push(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	hasOrigin, err := originRemoteExists(ctx, w.repoRoot)
	if err != nil {
		return fmt.Errorf("checking origin remote: %w", err)
	}

	if !hasOrigin {
		logger.Info("Could not push state branch because no origin remote is configured",
			"branch", w.branchName)
		return nil
	}

	if err := gitExecIn(ctx, w.Path, "push", "origin", w.branchName); err != nil {
		return fmt.Errorf("pushing state branch %q: %w", w.branchName, err)
	}
	return nil
}

func originRemoteExists(ctx context.Context, root string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = root
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func remoteBranchExists(ctx context.Context, root, branchName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--heads", "origin", branchName)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("git ls-remote --heads origin %s: %w: %s", branchName, err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()) != "", nil
}

func fetchRemoteBranch(ctx context.Context, root, branchName string) error {
	remoteRef := remoteTrackingRef(branchName)
	branchRef := "refs/heads/" + branchName
	return gitExecIn(ctx, root, "fetch", "origin", branchRef+":"+remoteRef)
}

func syncLocalBranchToRemote(ctx context.Context, root, branchName string) error {
	return gitExecIn(ctx, root, "branch", "--force", branchName, remoteTrackingRef(branchName))
}

func remoteTrackingRef(branchName string) string {
	return "refs/remotes/origin/" + branchName
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
