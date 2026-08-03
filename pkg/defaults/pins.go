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

import "log"

// resourceTypePins is the `resourceTypes` section of defaults.yaml keyed by
// namespace. Populated once at package init and read-only afterwards.
var resourceTypePins map[string]Pin

// loadPins populates the pin lookup from the parsed catalog. It is called once
// from the package init; see that function for the concurrency contract.
func loadPins(logger *log.Logger, pins []Pin) {
	resourceTypePins = make(map[string]Pin, len(pins))

	for _, pin := range pins {
		if pin.Name == "" || pin.Ref == "" {
			logger.Printf("defaults.yaml resourceTypes entry missing name or ref: %+v; skipping", pin)
			continue
		}
		resourceTypePins[pin.Name] = pin
	}
}

// ResourceTypePin returns the upstream revision that defaults.yaml pins the
// given resource-type namespace (e.g. "Radius.Compute") to. ok is false when
// the namespace has no `resourceTypes` entry — callers apply their own fallback
// rather than treating that as fatal, because a mis-configured build should
// still produce a working product (see pkg/cli/recipepack.resolveRecipeTag,
// which falls back to the "edge" OCI tag).
//
// Pin.Ref is the immutable commit SHA recorded by `make update-resource-types`
// and is safe to use directly as an OCI tag: commit SHAs match the OCI tag
// character set, and the resource-types-contrib publishing pipeline stamps that
// SHA on each artifact it publishes for the namespace. Pin.Tag is not — upstream
// release tags are namespaced with "/" (e.g. "Radius.Compute/v0.2.0").
func ResourceTypePin(namespace string) (Pin, bool) {
	pin, ok := resourceTypePins[namespace]
	return pin, ok
}
