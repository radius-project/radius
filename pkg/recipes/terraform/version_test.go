/*
Copyright 2026 The Radius Authors.

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

package terraform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTerraformVersionMatchesFile guarantees that the hard-coded
// terraformVersion default in version.go stays in sync with the
// .terraform-version file at the repository root.
func TestTerraformVersionMatchesFile(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to determine test file location")

	// Walk up from this file to the repo root (which contains go.mod).
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "could not locate repository root (go.mod)")
		dir = parent
	}

	contents, err := os.ReadFile(filepath.Join(dir, ".terraform-version"))
	require.NoError(t, err, "failed to read .terraform-version at repo root")

	require.Equal(t,
		strings.TrimSpace(string(contents)),
		terraformVersion,
		"terraformVersion default in version.go must match .terraform-version at repo root",
	)
}

// TestTerraformVersionMatchesHelmChart guarantees that the
// global.terraform.version default in deploy/Chart/values.yaml stays
// in sync with terraformVersion. The chart's terraform-init init
// container pre-downloads this version into the shared volume that
// the runtime (install.go::ensureGlobalTerraformBinary) consumes. If
// the chart default drifts from terraformVersion, the runtime's
// downloadAndInstallTerraform fallback installs a different version
// than the init container, so recipes behave differently between cold
// and warm starts (see #11880 / Test_TerraformRecipe_Context failure).
func TestTerraformVersionMatchesHelmChart(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to determine test file location")

	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "could not locate repository root (go.mod)")
		dir = parent
	}

	contents, err := os.ReadFile(filepath.Join(dir, "deploy", "Chart", "values.yaml"))
	require.NoError(t, err, "failed to read deploy/Chart/values.yaml")

	// Look for the literal line `    version: "<terraformVersion>"` inside
	// the global.terraform block. A literal substring check is sufficient
	// because the indentation uniquely identifies that nested key.
	expected := fmt.Sprintf("    version: %q", terraformVersion)
	require.Contains(t,
		string(contents),
		expected,
		"global.terraform.version in deploy/Chart/values.yaml must match terraformVersion in pkg/recipes/terraform/version.go (%q)",
		terraformVersion,
	)
}
