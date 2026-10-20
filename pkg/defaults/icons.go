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
	"crypto/sha256"
	"encoding/hex"
	"log"
	"path"
	"strings"

	manifestassets "github.com/radius-project/radius/deploy/manifest"
)

// Icon is the SVG bytes and SHA-256 hex hash of a resource-type icon.
type Icon struct {
	// Hash is the SHA-256 of Bytes, hex-encoded. Matches the `iconHash` field
	// stored on the resource-type record and returned in graph responses.
	Hash string
	// Bytes is the verbatim SVG UTF-8 content of the icon.
	Bytes []byte
}

var (
	defaultIcon  Icon
	builtInIcons map[string]Icon // key: fully-qualified resource type, e.g. "Radius.Compute/containers"
)

// loadIcons populates the icon lookups from the embedded assets. It is called
// once from the package init; see that function for the concurrency contract.
func loadIcons(logger *log.Logger, defaultRegistration []string) {
	// The default icon powers the fallback path for every registered type
	// without a per-type SVG. If the embedded asset is unreadable or empty we
	// log and leave defaultIcon zero-valued; DefaultIconHash then returns nil
	// and callers set iconHash to nil on their outputs.
	if svg, err := manifestassets.FS.ReadFile(manifestassets.DefaultIconPath); err != nil || len(svg) == 0 {
		logger.Printf("%s is unreadable or empty; default-icon fallback disabled", manifestassets.DefaultIconPath)
	} else {
		defaultIcon = Icon{Bytes: svg, Hash: hashOf(svg)}
	}

	// builtInIcons stays empty when defaults.yaml is unreadable; per-type
	// lookups return (Icon{}, false) and callers fall back to the default (or,
	// if the default is also unavailable, to nil).
	builtInIcons = map[string]Icon{}

	for _, fullyQualifiedType := range defaultRegistration {
		// defaults.yaml entries are "<namespace>/<typeName>"; the mirrored SVG
		// (if any) is at built-in-providers/self-hosted/<typeName>.svg. A type
		// without a paired SVG stays absent from the map — callers (static and
		// runtime graph) fall through to the default icon.
		_, typeName, ok := SplitResourceType(fullyQualifiedType)
		if !ok {
			logger.Printf("malformed defaults.yaml defaultRegistration entry %q; skipping", fullyQualifiedType)
			continue
		}

		b, err := manifestassets.FS.ReadFile(path.Join(manifestassets.BuiltInIconsDir, typeName+".svg"))
		if err != nil {
			// Missing SVG is not an error — many contributed types do not yet
			// ship an icon in resource-types-contrib, and the fallback is the
			// product default at graph-render time.
			continue
		}
		builtInIcons[fullyQualifiedType] = Icon{Bytes: b, Hash: hashOf(b)}
	}
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LookupIcon returns the icon for the given fully-qualified resource type
// (e.g. "Radius.Compute/containers"). ok is false when the type is not in
// defaults.yaml or its SVG has not yet been shipped in resource-types-contrib.
// Callers that want a guaranteed icon should fall through to DefaultIcon.
func LookupIcon(resourceType string) (Icon, bool) {
	icon, ok := builtInIcons[resourceType]
	return icon, ok
}

// DefaultIcon returns the Radius product default icon. Every Radius binary
// carries the same bytes, so a hash produced anywhere in the codebase is
// comparable to one produced by the control plane during resource-type
// registration. Both fields are zero-valued (empty Hash, nil Bytes) when the
// embedded default asset failed to load at init time; see the package doc for
// the "icon absence is not an error" contract.
func DefaultIcon() Icon {
	return defaultIcon
}

// DefaultIconHash returns a pointer to the product default icon's hash suitable
// for assigning directly to a resource-type record or graph node's IconHash
// field. Returns nil when the default is unavailable — callers should forward
// that nil to leave IconHash unset on their outputs (rather than storing a
// pointer to an empty string). This is the single spelling of the "graceful
// degradation" fallback used across the registration path, the runtime graph
// pipeline, and the static graph builder.
func DefaultIconHash() *string {
	if defaultIcon.Hash == "" {
		return nil
	}
	h := defaultIcon.Hash
	return &h
}

// IsDefaultIcon reports whether the given hex-encoded SHA-256 hash matches the
// product default icon's hash. Returns false when the given hash is empty or
// when the default is unavailable — safe to call unconditionally.
func IsDefaultIcon(hash string) bool {
	return hash != "" && hash == defaultIcon.Hash
}

// SplitResourceType splits a fully-qualified resource type of the form
// "<namespace>/<typeName>" (e.g. "Radius.Compute/containers") into its two
// parts. Returns ("", "", false) if the input does not match that shape
// (no separator, empty namespace, or empty type name). This is the format
// used by defaultRegistration in defaults.yaml and by every consumer of
// LookupIcon, and it is also how graph pipelines bucket resources by provider
// namespace before calling GetProviderSummary.
func SplitResourceType(resourceType string) (namespace, typeName string, ok bool) {
	slash := strings.Index(resourceType, "/")
	if slash <= 0 || slash == len(resourceType)-1 {
		return "", "", false
	}
	return resourceType[:slash], resourceType[slash+1:], true
}
