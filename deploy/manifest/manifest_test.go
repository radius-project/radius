/*
Copyright 2024 The Radius Authors.

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

package manifest

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Test_DefaultsYAML_NotEmpty asserts that defaults.yaml was embedded at
// build time. A zero-length result indicates go:embed failed silently or the
// file was renamed/moved without updating the //go:embed directive.
func Test_DefaultsYAML_NotEmpty(t *testing.T) {
	require.NotEmpty(t, DefaultsYAML(), "embedded defaults.yaml must not be empty")
	require.Contains(t, string(DefaultsYAML()), "defaultRegistration",
		"embedded defaults.yaml must contain the defaultRegistration key")
}

// Test_ParseDefaults_HasEntries asserts the embedded defaults.yaml parses and
// contains at least one default-registered resource type entry, each
// matching the documented "Radius.<Namespace>/<typeName>" shape.
func Test_ParseDefaults_HasEntries(t *testing.T) {
	d, err := ParseDefaults()
	require.NoError(t, err)
	require.NotEmpty(t, d.DefaultRegistration, "defaultRegistration must list at least one entry")

	for _, entry := range d.DefaultRegistration {
		require.True(t, strings.HasPrefix(entry, "Radius."),
			"entry %q must start with Radius.", entry)
		require.Contains(t, entry, "/",
			"entry %q must contain a / separator", entry)
	}
}
