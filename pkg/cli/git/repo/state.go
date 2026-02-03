// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

// Package repo provides Git repository detection and state operations.
package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/radius-project/radius/pkg/cli/git"
)

// RadiusDir is the name of the Radius configuration directory.
const RadiusDir = ".radius"

// IsGitRepository checks if the given directory is inside a Git repository.
func IsGitRepository(dir string) (bool, error) {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// IsGitWorkspace checks if the given directory is a Radius Git workspace.
func IsGitWorkspace(dir string) (bool, error) {
	isGit, err := IsGitRepository(dir)
	if err != nil {
		return false, err
	}
	if !isGit {
		return false, nil
	}

	radiusDir := filepath.Join(dir, RadiusDir)
	info, err := os.Stat(radiusDir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return info.IsDir(), nil
}

// GetRepositoryRoot returns the root directory of the Git repository.
func GetRepositoryRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", git.NewValidationError("not a Git repository", err.Error())
	}

	return strings.TrimSpace(string(output)), nil
}

// HasUncommittedChanges checks if there are uncommitted changes in the specified paths.
func HasUncommittedChanges(dir string, paths ...string) (bool, error) {
	args := []string{"status", "--porcelain"}
	args = append(args, paths...)

	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return false, err
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

// GetCurrentCommit returns the current HEAD commit SHA.
func GetCurrentCommit(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", git.NewValidationError("failed to get current commit", err.Error())
	}

	return strings.TrimSpace(string(output)), nil
}

// GetShortCommit returns the abbreviated current HEAD commit SHA.
func GetShortCommit(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", git.NewValidationError("failed to get current commit", err.Error())
	}

	return strings.TrimSpace(string(output)), nil
}

// ResolveCommit resolves a commit reference (SHA, tag, branch) to a full SHA.
func ResolveCommit(dir, ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return "", git.NewValidationError("invalid commit reference", ref)
	}

	return strings.TrimSpace(string(output)), nil
}

// StageFiles stages the specified files for commit.
func StageFiles(dir string, files ...string) error {
	if len(files) == 0 {
		return nil
	}

	args := append([]string{"add"}, files...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	if err := cmd.Run(); err != nil {
		return git.NewGeneralError("failed to stage files", err)
	}

	return nil
}

// CommitChanges creates a commit with the specified message.
func CommitChanges(dir, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir

	if err := cmd.Run(); err != nil {
		return git.NewGeneralError("failed to commit changes", err)
	}

	return nil
}

// GitInfo contains information about the current Git repository state.
type GitInfo struct {
	Root           string
	CommitSHA      string
	ShortSHA       string
	Branch         string
	HasUncommitted bool
}

// GetGitInfo retrieves information about the current Git repository state.
func GetGitInfo(dir string) (*GitInfo, error) {
	root, err := GetRepositoryRoot(dir)
	if err != nil {
		return nil, err
	}

	commitSHA, err := GetCurrentCommit(root)
	if err != nil {
		return nil, err
	}

	shortSHA, err := GetShortCommit(root)
	if err != nil {
		return nil, err
	}

	branch := getCurrentBranch(root)

	hasUncommitted, err := HasUncommittedChanges(root)
	if err != nil {
		return nil, err
	}

	return &GitInfo{
		Root:           root,
		CommitSHA:      commitSHA,
		ShortSHA:       shortSHA,
		Branch:         branch,
		HasUncommitted: hasUncommitted,
	}, nil
}

func getCurrentBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	branch := strings.TrimSpace(string(output))
	if branch == "HEAD" {
		return ""
	}

	return branch
}

// IsGitHubActions returns true if running in GitHub Actions environment.
func IsGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// GetGitHubSHA returns the commit SHA from GitHub Actions environment.
func GetGitHubSHA() string {
	return os.Getenv("GITHUB_SHA")
}
