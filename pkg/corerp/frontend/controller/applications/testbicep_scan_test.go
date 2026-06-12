// ------------------------------------------------------------
// Copyright 2026 The Radius Authors.
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

package applications

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_NoSharedDefaultEnvironmentInTestBicep enforces the rule that no test
// .bicep file may declare an Applications.Core/environments resource named
// 'default'. Mutating the shared `default` environment from a parallel test
// (e.g. setting its compute.namespace to a long string) leaks into other tests
// that target `default` and silently breaks them — most visibly by pushing the
// joined `<envNamespace>-<appName>` past Kubernetes' 63-character namespace
// limit. CI clusters are ephemeral so the bug usually hides there, but local
// `make debug-start` persists state in etcd/postgres across runs and exposes
// it. See pkg/corerp/frontend/controller/applications/updatefilter.go.
//
// If you have a legitimate need to test the default env, do it in a dedicated
// test that creates and tears down its own uniquely-named env.
func Test_NoSharedDefaultEnvironmentInTestBicep(t *testing.T) {
	repoRoot := findRepoRoot(t)
	testDir := filepath.Join(repoRoot, "test")

	// Matches:
	//   resource <ident> 'Applications.Core/environments@...' = {
	//     name: 'default'
	// Allows arbitrary whitespace and any resource-symbol name.
	envBlock := regexp.MustCompile(
		`(?s)resource\s+\w+\s+'Applications\.Core/environments@[^']+'\s*=\s*\{[^}]*?name:\s*'default'`)

	var offenders []string
	err := filepath.Walk(testDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".bicep") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if envBlock.Match(data) {
			rel, _ := filepath.Rel(repoRoot, path)
			offenders = append(offenders, rel)
		}
		return nil
	})
	require.NoError(t, err)

	require.Emptyf(t, offenders,
		"the following test bicep files declare an Applications.Core/environments named 'default', "+
			"which mutates the shared default env and breaks other parallel tests "+
			"(see Test_NoSharedDefaultEnvironmentInTestBicep doc comment):\n  %s",
		strings.Join(offenders, "\n  "))
}

// findRepoRoot walks up from this test file's directory until it finds a go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (no go.mod found above %s)", filepath.Dir(thisFile))
		}
		dir = parent
	}
}
