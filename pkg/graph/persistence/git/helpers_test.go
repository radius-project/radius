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

package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// runGit executes a command (typically "git ...") in dir and asserts success.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %v failed: %s", args, string(out))
	return string(out)
}

// initTestRepo creates a bare-minimum git repo in a temp dir with one commit so that Store
// operations (which run relative to a checkout) work.
func initTestRepo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping git-backed test in -short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("skipping git-backed test: git binary not found in PATH")
	}

	dir := t.TempDir()
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "config", "user.name", "test"},
		{"git", "config", "user.email", "test@test.com"},
	} {
		runGit(t, dir, args...)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0o644))
	runGit(t, dir, "git", "add", "-A")
	runGit(t, dir, "git", "commit", "-m", "init")
	return dir
}

// chdir switches to dir and registers a cleanup to switch back.
func chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(orig) })
}
