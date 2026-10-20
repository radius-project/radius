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

// Package graph provides primitives used by the rad CLI to build and persist
// application graph artifacts (modeled, planned and deployed) without
// touching the Radius control plane.
package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"github.com/radius-project/radius/pkg/cli/clierrors"
)

// nonAuthorableProperties are property keys removed from a resource before
// computing its diff hash. These fields are bound by the Radius environment
// at deploy time rather than authored by the developer, so including them
// would cause logically-equivalent resources to hash differently across
// environments.
var nonAuthorableProperties = map[string]struct{}{
	"provisioningState": {},
	"status":            {},
}

// diffHashPayload is the canonical structure that ComputeDiffHash hashes.
// encoding/json marshals map keys in sorted order, which together with the
// pre-sorted DependsOn slice yields a deterministic byte sequence regardless
// of map iteration order.
type diffHashPayload struct {
	Properties map[string]any `json:"properties"`
	DependsOn  []string       `json:"dependsOn"`
}

// ComputeDiffHash returns a stable "sha256:<hex>" digest computed over the
// authorable subset of properties and a sorted dependsOn list. The digest
// lets tooling classify resources as added, removed, modified or unchanged
// across two graphs of the same application without comparing every
// property.
func ComputeDiffHash(properties map[string]any, dependsOn ...string) (string, error) {
	authorable := make(map[string]any, len(properties))
	for k, v := range properties {
		if _, skip := nonAuthorableProperties[k]; skip {
			continue
		}
		authorable[k] = v
	}

	sorted := slices.Clone(dependsOn)
	slices.Sort(sorted)

	canonical, err := json.Marshal(diffHashPayload{Properties: authorable, DependsOn: sorted})
	if err != nil {
		return "", clierrors.MessageWithCause(err, "Failed to marshal canonical form for diff hash.")
	}

	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
