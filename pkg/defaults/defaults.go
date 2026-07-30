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

// Package defaults exposes what Radius ships as defaults: the resource types
// that are registered out of the box, the icons drawn for them, and the
// upstream resource-types-contrib revisions those defaults were built from.
//
// All of it is declared in deploy/manifest/defaults.yaml and its sibling SVG
// assets, which are embedded into every Radius binary by the assets-only
// package [github.com/radius-project/radius/deploy/manifest] (//go:embed cannot
// reach outside its own directory, and those files are deployment artifacts
// that the build tooling owns). This package parses those bytes once at init
// and serves them through the accessors below.
//
// # Icons
//
// Two consumers read icons:
//
//  1. The static (modeled) graph builder in `pkg/cli/graph` — the CLI binary
//     has no control plane to consult, so it resolves per-node iconHash values
//     from the embedded map.
//  2. The runtime graph pipeline in `pkg/corerp/frontend/controller/applications`
//     for substituting the default icon's bytes into the response's `icons`
//     map for types whose stored `iconHash` matches the default. Default icons
//     are the fallback for connected external cloud nodes that are not
//     registered in the local Radius resource-type registry. They are also used
//     when a user does not supply an icon for the resource type.
//
// Icon absence is never an error. Icons are cosmetic, so a missing or malformed
// one must not fail a graph request, refuse a resource-type registration, or
// crash the process. Three states are modeled, in fallback order:
//
//  1. Per-type SVG registered by the user (or shipped in
//     `built-in-providers/self-hosted/<typeName>.svg`) — use it.
//  2. Product default icon (embedded `default-icon.svg`) — use it when 1 is
//     unavailable.
//  3. Both unavailable — leave the node's `iconHash` unset (nil). Downstream
//     consumers render the node without an icon.
//
// The unavailable-default case is degenerate (only triggered if the build
// shipped a broken asset), so it is surfaced by logging to stderr at init time.
// Callers ask for a hash via [DefaultIconHash] (returns nil if none) and for
// bytes via [DefaultIcon] (empty Icon if none); both are safe to call
// unconditionally.
//
// # Pins
//
// The same file records which upstream revision each default was taken from.
// [ResourceTypePin] exposes those entries so release builds can address the
// artifacts published from exactly that revision — see `pkg/cli/recipepack`,
// which uses the pinned commit SHA as the OCI tag of each core recipe.
package defaults

import (
	"log"
	"os"

	"sigs.k8s.io/yaml"

	manifestassets "github.com/radius-project/radius/deploy/manifest"
)

// Pin records the upstream revision that defaults.yaml pins one entry to. The
// `resourceTypes` and `recipePacks` sections share this shape, so one type
// serves both.
type Pin struct {
	// Name identifies the entry: a resource-type namespace (e.g.
	// "Radius.Compute") under `resourceTypes`, or a recipe pack folder (e.g.
	// "azure") under `recipePacks`.
	Name string
	// Repo is the upstream repository the entry is fetched from, without a
	// scheme (e.g. "github.com/radius-project/resource-types-contrib").
	Repo string
	// Ref is the immutable commit SHA the entry is pinned to.
	Ref string
	// Tag is the upstream release tag Ref was resolved from (e.g.
	// "Radius.Compute/v0.2.0"), or empty when the pin came from the edge
	// channel. Note that tags are not valid OCI tags — they may contain "/".
	Tag string
}

// catalog mirrors deploy/manifest/defaults.yaml. Fields it does not declare are
// ignored by yaml.Unmarshal, so upstream can add sections without breaking init.
type catalog struct {
	ResourceTypes       []Pin    `json:"resourceTypes"`
	RecipePacks         []Pin    `json:"recipePacks"`
	DefaultRegistration []string `json:"defaultRegistration"`
}

// init loads the embedded catalog exactly once, before any exported function in
// this package can run.
//
// Go guarantees init completes on a single goroutine before main, so the maps
// and values it populates are immutable for the process lifetime and safe for
// concurrent reads from any caller (the CLI's static graph builder, the control
// plane's runtime graph pipeline, the icon endpoint, or the recipe pack builder)
// without further locking or sync.Once bookkeeping.
//
// A parse failure is logged and leaves the lookups empty rather than aborting:
// every accessor has a documented "not found" answer that callers already
// handle, so a broken asset degrades the product instead of killing it.
func init() {
	// The ucplog logger is not available here: init() runs at package-import
	// time, before any context.Context exists and before the ucp logger has
	// been configured.
	logger := log.New(os.Stderr, "defaults: ", log.LstdFlags)

	var parsed catalog
	if raw, err := manifestassets.FS.ReadFile(manifestassets.DefaultsYAMLPath); err != nil {
		logger.Printf("read defaults.yaml: %s; icon and pin lookups disabled", err)
	} else if err := yaml.Unmarshal(raw, &parsed); err != nil {
		logger.Printf("parse defaults.yaml: %s; icon and pin lookups disabled", err)
	}

	loadIcons(logger, parsed.DefaultRegistration)
	loadPins(logger, parsed.ResourceTypes)
}
