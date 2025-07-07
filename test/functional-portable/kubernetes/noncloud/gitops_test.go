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

package kubernetes_noncloud_test

import (
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
)

// GitOpsTestStep represents a logical step in a GitOps test.
// This includes:
//   - The path of the directory that contains the state of the repository
//     during the step.
//   - The list of expected Radius resources that should exist after execution
//     of the step.
type GitOpsTestStep struct {
	// The path of the test data directory.
	path string
	// The Radius resource group that resources are created in.
	resourceGroup string
	// The expected Radius resources that should be created on the cluster.
	expectedResources [][]string
	// The expected Radius resources that should not exist on the cluster.
	// This is used to verify that the resources were deleted.
	expectedResourcesToNotExist [][]string
}

// addFilesToRepository adds all files from the given path to the repository.
func addFilesToRepository(w *git.Worktree, fromPath, toPath string) error {
	return filepath.WalkDir(fromPath, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(fromPath, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(toPath, relPath)
		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}
		// Read contents of srcFile and write to destination.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		err = os.WriteFile(dstPath, data, 0644)
		if err != nil {
			return err
		}

		_, err = w.Add(relPath)
		return err
	})
}
