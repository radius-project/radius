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

package defaults

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResourceTypePin_KnownNamespaceReturnsPin pins the contract
// ResourceTypePin exposes: every namespace declared under `resourceTypes` in
// defaults.yaml resolves to an entry carrying a non-empty commit SHA and the
// upstream repository it came from. This is also the guard that catches a
// rename of the section or its fields — the loader tolerates an unknown shape
// silently, so only a test notices.
func TestResourceTypePin_KnownNamespaceReturnsPin(t *testing.T) {
	pin, ok := ResourceTypePin("Radius.Compute")
	require.True(t, ok, "Radius.Compute must have a resourceTypes entry in defaults.yaml")
	assert.Equal(t, "Radius.Compute", pin.Name)
	assert.NotEmpty(t, pin.Ref, "resourceTypes[Radius.Compute].ref must be a non-empty commit SHA")
	assert.NotEmpty(t, pin.Repo, "resourceTypes[Radius.Compute].repo must name the upstream repository")
}

// TestResourceTypePin_UnknownNamespaceReturnsFalse covers the fall-through case
// callers rely on: an unregistered namespace reports not-found so they can
// apply their own fallback (see pkg/cli/recipepack.resolveRecipeTag).
func TestResourceTypePin_UnknownNamespaceReturnsFalse(t *testing.T) {
	_, ok := ResourceTypePin("MyCompany.Test")
	assert.False(t, ok)

	_, ok = ResourceTypePin("")
	assert.False(t, ok)
}
