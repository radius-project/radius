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

package kubernetes_test

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
	// The expected Radius resources that should be created on the cluster.
	expectedResources [][]string
}

// addFilesToRepository adds all files from the given path to the repository.
func addFilesToRepository(worktree *git.Worktree, path string) error {
	_ = filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		_, err = worktree.Add(path)

		return err
	})

	return nil
}
