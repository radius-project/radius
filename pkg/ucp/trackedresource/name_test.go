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

package trackedresource

import (
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

var (
	testID = resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/Applications.Core/applications/test-app")
)

func Test_NameFor(t *testing.T) {
	name := NameFor(testID)
	require.Equal(t, "test-app-578debcee1fa4c9d11de5dd86eb2945e08b4b12f", name)
}

func Test_IDFor(t *testing.T) {
	id := IDFor(testID)
	require.Equal(t, resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/System.Resources/resources/test-app-578debcee1fa4c9d11de5dd86eb2945e08b4b12f"), id)
}

// Test_LegacyNameFor and Test_LegacyIDFor lock the legacy SHA-1 values so that Radius can
// continue to locate tracked resource entries written by older versions during the migration
// to SHA-256. See https://github.com/radius-project/radius/issues/8084.
func Test_LegacyNameFor(t *testing.T) {
	name := LegacyNameFor(testID)
	require.Equal(t, "test-app-303153687ee5adbcf353bc6c2caa4373f31e04c6", name)
}

func Test_LegacyIDFor(t *testing.T) {
	id := LegacyIDFor(testID)
	require.Equal(t, resources.MustParse("/planes/radius/local/resourceGroups/test-group/providers/System.Resources/resources/test-app-303153687ee5adbcf353bc6c2caa4373f31e04c6"), id)
}

func Test_TrackingNameCompatibility(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		nameFor  func(resources.ID) string
		idFor    func(resources.ID) resources.ID
		expected string
	}{
		{"current", NameFor, IDFor, "test-app-578debcee1fa4c9d11de5dd86eb2945e08b4b12f"},
		{"legacy", LegacyNameFor, LegacyIDFor, "test-app-303153687ee5adbcf353bc6c2caa4373f31e04c6"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			uppercase := resources.MustParse("/planes/radius/LOCAL/resourceGroups/TEST-GROUP/providers/Applications.Core/applications/TEST-APP")
			require.Equal(t, tt.expected, tt.nameFor(uppercase))
			require.Equal(t, tt.expected, tt.idFor(uppercase).Name())
			require.Empty(t, tt.nameFor(resources.ID{}))
			require.True(t, tt.idFor(resources.ID{}).IsEmpty())

			longID := resources.MustParse(testID.RootScope() + "/providers/Applications.Core/applications/" + strings.Repeat("a", 80))
			name := tt.nameFor(longID)
			require.Len(t, name, 63)
			require.Regexp(t, `^a{22}-[0-9a-f]{40}$`, name)
			require.Equal(t, name, tt.idFor(longID).Name())
		})
	}
}
