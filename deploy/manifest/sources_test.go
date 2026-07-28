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

package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSourceRef_KnownNamespaceReturnsRef pins the contract SourceRef
// exposes: every namespace declared under `sources` in defaults.yaml
// must resolve to a non-empty ref, and an unknown namespace must
// return ("", false) so callers can apply their own fallback (see
// pkg/cli/recipepack.resolveRecipeTag).
func TestSourceRef_KnownNamespaceReturnsRef(t *testing.T) {
	ref, ok := SourceRef("Radius.Compute")
	require.True(t, ok, "Radius.Compute must have a sources entry in defaults.yaml")
	assert.NotEmpty(t, ref, "sources[Radius.Compute].ref must be a non-empty commit SHA")
}

func TestSourceRef_UnknownNamespaceReturnsFalse(t *testing.T) {
	_, ok := SourceRef("MyCompany.Test")
	assert.False(t, ok)

	_, ok = SourceRef("")
	assert.False(t, ok)
}
