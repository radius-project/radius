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
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultBranch is the default orphan branch name for Radius state.
	DefaultBranch = "radius-state"
)

// OrphanBranchExists checks if the named branch exists in the local git repository.
func OrphanBranchExists(ctx context.Context, branchName string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", branchName)
	return cmd.Run() == nil
}

// CommitState stages and commits the state directory contents to an orphan branch,
// then switches back to the original branch.
//
// This is designed for use in a GitHub Actions runner where the checkout is always
// clean — no stashing is required. The state files in stateDir are untracked on the
// current branch and will survive any branch switch.
//
// Note: This does NOT push to a remote. The calling workflow must run
// 'git push origin radius-state' after this command returns.
func CommitState(ctx context.Context, stateDir, branchName string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	originalBranch, err := currentBranch(ctx)
	if err != nil {
		return fmt.Errorf("failed to determine current git branch: %w", err)
	}

	defer func() {
		_ = gitExec(ctx, "checkout", originalBranch)
	}()

	if OrphanBranchExists(ctx, branchName) {
		logger.Info("Switching to existing state branch", "branch", branchName)
		if err := gitExec(ctx, "checkout", branchName); err != nil {
			return fmt.Errorf("failed to checkout branch %q: %w", branchName, err)
		}
	} else {
		logger.Info("Creating orphan state branch", "branch", branchName)
		if err := gitExec(ctx, "checkout", "--orphan", branchName); err != nil {
			return fmt.Errorf("failed to create orphan branch %q: %w", branchName, err)
		}
		// Clear the index — ignore errors when the index is already empty.
		_ = gitExec(ctx, "rm", "-rf", "--cached", ".")
	}

	if err := gitExec(ctx, "add", "--force", stateDir); err != nil {
		return fmt.Errorf("failed to stage state directory %q: %w", stateDir, err)
	}

	msg := fmt.Sprintf("radius state backup %s", time.Now().UTC().Format(time.RFC3339))
	if err := gitExec(ctx, "commit", "-m", msg, "--allow-empty"); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}

	logger.Info("State committed to orphan branch", "branch", branchName, "message", msg)
	return nil
}

// RestoreState checks out the state directory files from the orphan branch into the
// current working tree without switching branches. The files are placed in the working
// directory but are NOT committed to the current branch.
func RestoreState(ctx context.Context, stateDir, branchName string) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	if !OrphanBranchExists(ctx, branchName) {
		logger.Info("State branch does not exist, skipping restore", "branch", branchName)
		return nil
	}

	logger.Info("Restoring state from orphan branch", "branch", branchName, "stateDir", stateDir)
	if err := gitExec(ctx, "checkout", branchName, "--", stateDir); err != nil {
		return fmt.Errorf("failed to restore state from branch %q: %w", branchName, err)
	}

	// Unstage the restored files so they don't appear as staged changes on the current branch.
	_ = gitExec(ctx, "reset", "HEAD", "--", stateDir)

	logger.Info("State restored from orphan branch", "branch", branchName)
	return nil
}

func currentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}

func gitExec(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}

	return nil
}
