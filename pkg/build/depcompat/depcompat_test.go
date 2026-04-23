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

package depcompat

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_K8sDependencyCompatibility verifies that the k8s.io and controller-runtime
// dependency versions are compatible. This test was added as a regression test after
// a dependency bump to k8s.io v0.36.0 broke the build because controller-runtime
// v0.23.3 was not compatible with k8s.io v0.36.0.
//
// The k8s.io/client-go library must use the same minor version as the
// sigs.k8s.io/controller-runtime library expects. When bumping k8s.io/*
// dependencies to a new minor version (e.g., 0.35.x → 0.36.x), the
// controller-runtime dependency must also be bumped to a compatible version.
func Test_K8sDependencyCompatibility(t *testing.T) {
	goModPath := findGoMod(t)
	content, err := os.ReadFile(goModPath)
	require.NoError(t, err, "failed to read go.mod")

	clientGoVersion := findModuleVersion(string(content), "k8s.io/client-go")
	require.NotEmpty(t, clientGoVersion, "k8s.io/client-go not found in go.mod")

	controllerRuntimeVersion := findModuleVersion(string(content), "sigs.k8s.io/controller-runtime")
	require.NotEmpty(t, controllerRuntimeVersion, "sigs.k8s.io/controller-runtime not found in go.mod")

	clientGoMinor := extractMinorVersion(t, clientGoVersion)
	crMinor := extractMinorVersion(t, controllerRuntimeVersion)

	// The k8s.io minor version and controller-runtime minor version must follow
	// the established compatibility matrix. controller-runtime vX.Y.Z is
	// compatible with k8s.io v0.(Y+12).Z (e.g., controller-runtime v0.23.x ↔ k8s.io v0.35.x).
	//
	// This test catches the case where k8s.io is bumped without a matching
	// controller-runtime bump (or vice versa).
	expectedK8sMinor := crMinor + 12
	require.Equalf(t, expectedK8sMinor, clientGoMinor,
		"k8s.io/client-go minor version (%d from %s) is not compatible with "+
			"sigs.k8s.io/controller-runtime minor version (%d from %s). "+
			"Expected k8s.io minor = controller-runtime minor + 12. "+
			"When bumping k8s.io/* dependencies, also bump controller-runtime to a compatible version.",
		clientGoMinor, clientGoVersion, crMinor, controllerRuntimeVersion)
}

// findGoMod locates the root go.mod file relative to this test file.
func findGoMod(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller info")

	dir := filepath.Dir(filename)
	for {
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return goMod
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

// findModuleVersion finds the version of a module in go.mod content.
func findModuleVersion(goModContent, modulePath string) string {
	for _, line := range strings.Split(goModContent, "\n") {
		line = strings.TrimSpace(line)
		// Skip comments and indirect deps marker
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "module") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] == modulePath {
			return parts[1]
		}
	}
	return ""
}

// extractMinorVersion extracts the minor version number from a semver-like version string.
// For example, "v0.35.4" returns 35, "v0.23.3" returns 23.
func extractMinorVersion(t *testing.T, version string) int {
	t.Helper()

	// Strip leading 'v' if present
	version = strings.TrimPrefix(version, "v")

	parts := strings.Split(version, ".")
	require.GreaterOrEqualf(t, len(parts), 2, "version %q does not have a minor component", version)

	minor := 0
	for _, c := range parts[1] {
		require.True(t, c >= '0' && c <= '9', "non-numeric character in minor version: %q", version)
		minor = minor*10 + int(c-'0')
	}

	return minor
}
