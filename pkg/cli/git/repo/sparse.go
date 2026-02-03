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

package repo

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/git"
)

// ResourceTypesRepoURL is the default repository for Resource Types.
const ResourceTypesRepoURL = "https://github.com/radius-project/resource-types-contrib.git"

// ResourceTypesPath is the subdirectory in the repository containing Resource Types.
const ResourceTypesPath = "types"

// SparseCheckout performs a sparse checkout of a Git repository.
// This is used to fetch only the Resource Types directory from the contrib repo.
type SparseCheckout struct {
	// RepoURL is the Git repository URL to clone.
	RepoURL string

	// Branch is the branch or tag to checkout (default: main).
	Branch string

	// SparsePath is the subdirectory to checkout.
	SparsePath string

	// TargetDir is the directory where the checkout will be placed.
	TargetDir string
}

// NewResourceTypesSparseCheckout creates a SparseCheckout configured for Resource Types.
func NewResourceTypesSparseCheckout(targetDir string) *SparseCheckout {
	return &SparseCheckout{
		RepoURL:    ResourceTypesRepoURL,
		Branch:     "main",
		SparsePath: ResourceTypesPath,
		TargetDir:  targetDir,
	}
}

// Execute performs the sparse checkout operation.
// Creates a shallow clone with only the specified path populated.
func (sc *SparseCheckout) Execute() error {
	// Create a temporary directory for the clone
	tempDir, err := os.MkdirTemp("", "radius-sparse-*")
	if err != nil {
		return git.NewGeneralError("failed to create temp directory", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize empty git repository
	if err := runGitCommand(tempDir, "init"); err != nil {
		return git.NewGeneralError("failed to initialize git repository", err)
	}

	// Add remote
	if err := runGitCommand(tempDir, "remote", "add", "origin", sc.RepoURL); err != nil {
		return git.NewGeneralError("failed to add remote", err)
	}

	// Enable sparse checkout
	if err := runGitCommand(tempDir, "config", "core.sparseCheckout", "true"); err != nil {
		return git.NewGeneralError("failed to enable sparse checkout", err)
	}

	// Configure sparse checkout paths
	sparseFile := filepath.Join(tempDir, ".git", "info", "sparse-checkout")
	if err := os.WriteFile(sparseFile, []byte(sc.SparsePath+"\n"), 0644); err != nil {
		return git.NewGeneralError("failed to write sparse-checkout file", err)
	}

	// Fetch the branch with depth 1 (shallow clone)
	branch := sc.Branch
	if branch == "" {
		branch = "main"
	}
	if err := runGitCommand(tempDir, "fetch", "--depth=1", "origin", branch); err != nil {
		return git.NewValidationError("failed to fetch Resource Types", "check network connectivity and repository access")
	}

	// Checkout the branch
	if err := runGitCommand(tempDir, "checkout", "FETCH_HEAD"); err != nil {
		return git.NewGeneralError("failed to checkout", err)
	}

	// Ensure target directory exists
	if err := os.MkdirAll(sc.TargetDir, 0755); err != nil {
		return git.NewGeneralError("failed to create target directory", err)
	}

	// Copy sparse checkout content to target
	sourcePath := filepath.Join(tempDir, sc.SparsePath)
	if err := copyDirectory(sourcePath, sc.TargetDir); err != nil {
		return git.NewGeneralError("failed to copy Resource Types", err)
	}

	return nil
}

// runGitCommand executes a git command in the specified directory.
func runGitCommand(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stderr = nil // Suppress stderr for cleaner output
	return cmd.Run()
}

// copyDirectory recursively copies a directory from src to dst.
func copyDirectory(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return err
			}
			if err := copyDirectory(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	content, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, content, 0644)
}
