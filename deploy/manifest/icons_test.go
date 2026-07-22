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
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefault_HasNonEmptyBytesAndStableHash pins two invariants that the rest
// of the icon story depends on: the embedded default icon actually contains
// bytes, and the hash returned by Default() is the SHA-256 of exactly those
// bytes. If someone edits default-icon.svg the bytes/hash both change together
// — the test still passes — but corrupted or missing content is caught.
func TestDefault_HasNonEmptyBytesAndStableHash(t *testing.T) {
	def := Default()
	require.NotEmpty(t, def.Bytes, "default-icon.svg must not be empty")

	sum := sha256.Sum256(def.Bytes)
	assert.Equal(t, hex.EncodeToString(sum[:]), def.Hash)
}

// TestLookup_KnownBuiltInReturnsIcon covers the happy path: a type that
// ships an SVG in resource-types-contrib is present in the map, its bytes
// are non-empty, and its hash is the SHA-256 of those bytes. We use
// containers because its SVG is present in every dev/self-hosted mirror
// today.
func TestLookup_KnownBuiltInReturnsIcon(t *testing.T) {
	icon, ok := Lookup("Radius.Compute/containers")
	require.True(t, ok, "Radius.Compute/containers should have an embedded icon; if this fails, run make sync-resource-types")
	require.NotEmpty(t, icon.Bytes)

	sum := sha256.Sum256(icon.Bytes)
	assert.Equal(t, hex.EncodeToString(sum[:]), icon.Hash)

	// Built-in icons must not accidentally match the default. If they do,
	// static-graph consumers can't distinguish "this is the containers icon"
	// from "this type has no icon"; the whole point of the map is to give
	// distinct hashes for distinct types.
	assert.NotEqual(t, Default().Hash, icon.Hash)
}

// TestLookup_UnknownTypeReturnsFalse covers the fall-through case: user-
// defined types and external cloud namespaces (Microsoft.Storage/*, etc.)
// are never in the built-in map, and callers must handle that by falling
// through to Default().
func TestLookup_UnknownTypeReturnsFalse(t *testing.T) {
	_, ok := Lookup("MyCompany.Test/widgets")
	assert.False(t, ok)

	_, ok = Lookup("Microsoft.Storage/storageAccounts")
	assert.False(t, ok)

	// Malformed input must also return false rather than a partial match.
	_, ok = Lookup("")
	assert.False(t, ok)
	_, ok = Lookup("no-slash")
	assert.False(t, ok)
}

// TestIsDefault_MatchesDefaultHash locks in the semantics: IsDefault is true
// exactly for the default hash and false for anything else (including empty
// input and a built-in type's hash).
func TestIsDefault_MatchesDefaultHash(t *testing.T) {
	assert.True(t, IsDefault(Default().Hash))

	assert.False(t, IsDefault(""))
	assert.False(t, IsDefault("not-a-real-hash"))

	if icon, ok := Lookup("Radius.Compute/containers"); ok {
		assert.False(t, IsDefault(icon.Hash))
	}
}

// TestEmbeddedIcons_PassValidateIcon runs every icon this package ships —
// the product default and every per-type SVG under
// built-in-providers/self-hosted/ — through the same datamodel.ValidateIcon
// rules that gate user-attached icons on the CLI and control-plane ingress
// paths.
//
// Purpose: the embedded icons are hashed at init and served verbatim by the
// icon endpoint and the graph inline-icons map. They are never routed
// through ValidateIcon at runtime (hashing skips the check), so a
// malformed, oversized, script-bearing, or otherwise-hostile SVG merged
// into deploy/manifest would ship to production undetected. This test is
// the CI gate that prevents that class of regression.
//
// Rationale: catching this at unit-test time is strictly cheaper than
// wiring ValidateIcon into the go:embed init path, which would either
// panic the process at import (bad) or silently drop icons (worse). The
// icons are a build-time constant, so a build-time test is the right
// enforcement point.
func TestEmbeddedIcons_PassValidateIcon(t *testing.T) {
	def := Default()
	require.NotEmpty(t, def.Bytes, "default-icon.svg must be embedded")
	require.NoError(t, datamodel.ValidateIcon(def.Bytes), "default-icon.svg must pass ValidateIcon")

	require.NotEmpty(t, builtIns, "no built-in icons loaded; check defaults.yaml and built-in-providers/self-hosted/")
	for typeName, icon := range builtIns {
		t.Run(typeName, func(t *testing.T) {
			assert.NoError(t, datamodel.ValidateIcon(icon.Bytes),
				"embedded icon for %s (built-in-providers/self-hosted/) must pass ValidateIcon", typeName)
		})
	}
}

// both by this package's init() to bind SVGs to their entries in defaults.yaml
// and by graph pipelines to bucket resources by provider namespace before
// calling GetProviderSummary.
func TestSplitResourceType(t *testing.T) {
	cases := []struct {
		in          string
		namespace   string
		typeName    string
		ok          bool
		explanation string
	}{
		{"Radius.Compute/containers", "Radius.Compute", "containers", true, "well-formed"},
		{"Radius.Core/applications", "Radius.Core", "applications", true, "well-formed built-in"},
		{"", "", "", false, "empty input"},
		{"NoSlash", "", "", false, "missing separator"},
		{"/containers", "", "", false, "empty namespace"},
		{"Radius.Compute/", "", "", false, "empty type name"},
	}
	for _, c := range cases {
		t.Run(c.explanation, func(t *testing.T) {
			ns, tn, ok := SplitResourceType(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.namespace, ns)
			assert.Equal(t, c.typeName, tn)
		})
	}
}
