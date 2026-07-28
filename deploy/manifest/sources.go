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

// This file exposes the `sources:` metadata declared in defaults.yaml —
// the per-namespace pin (currently a resource-types-contrib commit SHA)
// that `make update-resource-types` rewrites when the vendored manifests
// are refreshed. The rest of the manifest package deals with icon
// assets; sources are a distinct concern that shares only the embedded
// `defaults.yaml` file, so they live in their own source file rather
// than crowding icons.go.

package manifest

import (
	"log"
	"os"

	"sigs.k8s.io/yaml"
)

// sourceRefs maps a resource-type namespace (e.g. "Radius.Compute") to
// the immutable ref recorded in defaults.yaml under `sources`. It is
// populated once at package init and treated as read-only afterwards,
// mirroring the concurrency model of the `builtIns` map in icons.go.
var sourceRefs map[string]string

// sourcesFile matches the subset of defaults.yaml consumed here. Other
// top-level fields (defaultRegistration, plus future additions) are
// tolerated by yaml.Unmarshal and ignored.
type sourcesFile struct {
	Sources []sourcePin `json:"sources"`
}

// sourcePin matches one entry of the `sources:` list in defaults.yaml.
// Only Namespace and Ref are consumed; Repo is tolerated so callers do
// not need to know about it and update-resource-types can add new
// fields (checksums, versioned artifacts, etc.) without breaking init.
type sourcePin struct {
	Namespace string `json:"namespace"`
	Repo      string `json:"repo"`
	Ref       string `json:"ref"`
}

// init loads the source-pin map from the embedded defaults.yaml exactly
// once. Go guarantees this runs on a single goroutine before main, so
// sourceRefs is immutable for the process lifetime and safe for
// concurrent reads. A parse failure or a malformed entry logs to stderr
// and leaves sourceRefs empty — callers apply their own fallback (see
// pkg/cli/recipepack.resolveRecipeTag) rather than treating a missing
// entry as fatal.
func init() {
	logger := log.New(os.Stderr, "manifest: ", log.LstdFlags)

	sourceRefs = map[string]string{}

	var file sourcesFile
	if err := yaml.Unmarshal(defaultsYAMLBytes, &file); err != nil {
		logger.Printf("parse defaults.yaml sources: %s; SourceRef lookup disabled", err)
		return
	}

	for _, src := range file.Sources {
		if src.Namespace == "" || src.Ref == "" {
			logger.Printf("defaults.yaml sources entry missing namespace or ref: %+v; skipping", src)
			continue
		}
		sourceRefs[src.Namespace] = src.Ref
	}
}

// SourceRef returns the pinned resource-types-contrib commit SHA for
// the given resource-type namespace (e.g. "Radius.Compute"), as
// declared in defaults.yaml under `sources`. ok is false when the
// namespace has no entry — callers (currently the CLI's default recipe
// pack builder in pkg/cli/recipepack) fall back to a safe non-pinned
// tag such as "edge" so a mis-configured build still installs recipes
// rather than failing.
//
// The returned string is the immutable ref recorded by
// `make update-resource-types` and is safe to use directly as an OCI
// tag: commit SHAs match the OCI tag character set and are the tag the
// resource-types-contrib publishing pipeline stamps on each recipe
// artifact for the corresponding namespace.
func SourceRef(namespace string) (string, bool) {
	ref, ok := sourceRefs[namespace]
	return ref, ok
}
