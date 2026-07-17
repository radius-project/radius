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

// Package manifest exposes the Radius product-shipped resource-type icon
// assets. The SVG bytes and the defaults.yaml catalog live in this directory
// as the canonical source of truth for both the runtime `make sync-resource-types`
// pipeline and this Go package; go:embed cannot reference paths above its own
// directory, so the Go file sits beside the assets rather than mirroring them.
//
// Two consumers use this package:
//
//  1. The static (modeled) graph builder in `pkg/cli/graph` — the CLI binary
//     has no control plane to consult, so it resolves per-node iconHash values
//     from the embedded map.
//  2. The runtime graph pipeline in `pkg/corerp/frontend/controller/applications`
//     for substituting the default icon's bytes into the response's `icons`
//     map for types whose stored `iconHash` matches the default. default icons are
//     the fallback for connected external cloud nodes that are not registered in
//     the local Radius resource-type registry. They are also used when user
//     does not supply an icon for the resource type.
//
// # Design decision: icon absence is not an error
//
// Icons are cosmetic. A missing or malformed icon is never a reason to fail
// a graph request, refuse a resource-type registration, or crash the
// process. This package models three states, in fallback order:
//
//  1. Per-type SVG registered by the user (or shipped in
//     `built-in-providers/self-hosted/<typeName>.svg`) — use it.
//  2. Product default icon (embedded `default-icon.svg`) — use it when 1 is
//     unavailable.
//  3. Both unavailable — leave the node's `iconHash` unset (nil). Downstream
//     consumers render the node without an icon.
//
// The unavailable-default case is degenerate (only triggered if the build
// shipped a broken asset), so we surface it by logging to stderr at init
// time. Callers ask for a hash via `DefaultHash` (returns nil if none) and
// for bytes via `Default().Bytes` (empty slice if none); both are safe to
// call unconditionally without a nil check on the package itself.
package manifest

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"log"
	"os"
	"path"
	"strings"

	"sigs.k8s.io/yaml"
)

//go:embed default-icon.svg
var defaultIconBytes []byte

//go:embed defaults.yaml
var defaultsYAMLBytes []byte

//go:embed built-in-providers/self-hosted/*.svg
var builtInIconsFS embed.FS

// Icon is the SVG bytes and SHA-256 hex hash of a resource-type icon.
type Icon struct {
	// Hash is the SHA-256 of Bytes, hex-encoded. Matches the `iconHash` field
	// stored on the resource-type record and returned in graph responses.
	Hash string
	// Bytes is the verbatim SVG UTF-8 content of the icon.
	Bytes []byte
}

var (
	defaultIcon Icon
	builtIns    map[string]Icon // key: fully-qualified resource type, e.g. "Radius.Compute/containers"
)

// defaultTypes matches the shape of defaults.yaml.
type defaultTypes struct {
	DefaultRegistration []string `json:"defaultRegistration"`
}

// init loads the embedded icon catalog exactly once, before any exported
// function in this package can run.
//
// Go guarantees init completes on a single goroutine before main, so the
// resulting `defaultIcon` value and `builtIns` map are immutable for the
// process lifetime and safe for concurrent reads from any caller (the
// CLI's static graph builder, the control plane's runtime graph pipeline,
// or the icon endpoint) without further locking or sync.Once bookkeeping.
func init() {
	// ucplog logger is not available since init() runs at package-import time,
	// before any context.Context exists and before the ucp logger has been configured
	logger := log.New(os.Stderr, "manifest: ", log.LstdFlags)

	// The default icon powers the fallback path for every registered type
	// without a per-type SVG. If the embedded asset is empty we log and
	// leave defaultIcon zero-valued; DefaultHash then returns nil and
	// callers set iconHash to nil on their outputs.
	if len(defaultIconBytes) == 0 {
		logger.Println("default-icon.svg is empty; default-icon fallback disabled")
	} else {
		defaultIcon = Icon{Bytes: defaultIconBytes, Hash: hashOf(defaultIconBytes)}
	}

	// builtIns starts empty even if defaults.yaml is unreadable; per-type
	// lookups return (Icon{}, false) and callers fall back to the default
	// (or, if the default is also unavailable, to nil).
	builtIns = map[string]Icon{}

	var catalog defaultTypes
	if err := yaml.Unmarshal(defaultsYAMLBytes, &catalog); err != nil {
		logger.Printf("parse defaults.yaml: %s; per-type icon lookup disabled", err)
		return
	}

	for _, fullyQualifiedType := range catalog.DefaultRegistration {
		// defaults.yaml entries are "<namespace>/<typeName>"; the mirrored
		// SVG (if any) is at built-in-providers/self-hosted/<typeName>.svg.
		// A type without a paired SVG stays absent from the map — callers
		// (static and runtime graph) fall through to the default icon.
		_, typeName, ok := SplitResourceType(fullyQualifiedType)
		if !ok {
			logger.Printf("malformed defaults.yaml entry %q; skipping", fullyQualifiedType)
			continue
		}
		svgPath := path.Join("built-in-providers/self-hosted", typeName+".svg")
		b, err := builtInIconsFS.ReadFile(svgPath)
		if err != nil {
			// Missing SVG is not an error — many contributed types do not
			// yet ship an icon in resource-types-contrib, and the fallback
			// is the product default at graph-render time.
			continue
		}
		builtIns[fullyQualifiedType] = Icon{Bytes: b, Hash: hashOf(b)}
	}
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Lookup returns the icon for the given fully-qualified resource type
// (e.g. "Radius.Compute/containers"). ok is false when the type is not in
// defaults.yaml or its SVG has not yet been shipped in resource-types-contrib.
// Callers that want a guaranteed icon should fall through to Default.
func Lookup(resourceType string) (Icon, bool) {
	icon, ok := builtIns[resourceType]
	return icon, ok
}

// Default returns the Radius product default icon. Every Radius binary carries
// the same bytes, so a hash produced anywhere in the codebase is comparable to
// one produced by the control plane during resource-type registration. Both
// fields are zero-valued (empty Hash, nil Bytes) when the embedded default
// asset failed to load at init time; see the package doc for the "icon
// absence is not an error" contract.
func Default() Icon {
	return defaultIcon
}

// DefaultHash returns a pointer to the product default icon's hash suitable
// for assigning directly to a resource-type record or graph node's IconHash
// field. Returns nil when the default is unavailable — callers should
// forward that nil to leave IconHash unset on their outputs (rather than
// storing a pointer to an empty string). This is the single spelling of the
// "graceful degradation" fallback used across the registration path, the
// runtime graph pipeline, and the static graph builder.
func DefaultHash() *string {
	if defaultIcon.Hash == "" {
		return nil
	}
	h := defaultIcon.Hash
	return &h
}

// IsDefault reports whether the given hex-encoded SHA-256 hash matches the
// product default icon's hash. Returns false when the given hash is empty
// or when the default is unavailable — safe to call unconditionally.
func IsDefault(hash string) bool {
	return hash != "" && hash == defaultIcon.Hash
}

// SplitResourceType splits a fully-qualified resource type of the form
// "<namespace>/<typeName>" (e.g. "Radius.Compute/containers") into its two
// parts. Returns ("", "", false) if the input does not match that shape
// (no separator, empty namespace, or empty type name). This is the format
// used as the map key in defaults.yaml and by every consumer of Lookup.
func SplitResourceType(resourceType string) (namespace, typeName string, ok bool) {
	slash := strings.Index(resourceType, "/")
	if slash <= 0 || slash == len(resourceType)-1 {
		return "", "", false
	}
	return resourceType[:slash], resourceType[slash+1:], true
}
