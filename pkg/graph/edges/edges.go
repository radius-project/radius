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

package edges

import (
	"sort"
	"strings"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
)

// ExcludedResourceTypes lists resource types that are never valid
// endpoints for a Dependency edge. Both the CLI static-graph builder
// (pkg/cli/graph) and the Radius.Core preview runtime handler
// (pkg/corerp/frontend/controller/applications/v20250801preview) share
// this policy so the two graphs surface an identical set of edges for
// the same input.
//
// The types listed here are structural containers (applications,
// environments, recipe packs) — Bicep authors typically write
// `dependsOn` against them incidentally, but they are not resources
// the graph is meant to visualise as endpoints.
var ExcludedResourceTypes = map[string]struct{}{
	"Applications.Core/applications": {},
	"Applications.Core/environments": {},
	"Radius.Core/applications":       {},
	"Radius.Core/environments":       {},
	"Radius.Core/recipePacks":        {},
}

// MergeDependencyEdges overlays caller-supplied Dependency edges onto
// an already-built ApplicationGraphResponse. For each entry
// dependsOnInfo[source] = [target, ...] the function adds an outbound
// Kind: Dependency edge on source -> target (and mirrors it as an
// inbound edge on target) provided that:
//
//   - Both source and target exist in graph.Resources.
//   - Neither source nor target has a Type in ExcludedResourceTypes.
//   - source does not already carry any outbound edge to target
//     (Connection wins; a Dependency the caller sent twice collapses).
//   - The entry itself is well-formed (Outbound + Kind: Dependency).
//
// ID matching is case-insensitive because Radius/ARM resource IDs are
// canonically case-insensitive but casing varies in practice
// (server-returned IDs use "resourcegroups" while cli.RequireScope
// produces "resourceGroups"). Emitted edges reference the canonical ID
// carried on graph.Resources so downstream consumers can look up
// either endpoint by exact-string match against the resources list.
//
// Every resource's Connections slice is re-sorted at the end by
// (Direction, Kind, ID) so downstream diff/hash consumers see a stable
// order.
//
// The function is a no-op when graph is nil or dependsOnInfo is empty.
func MergeDependencyEdges(
	graph *corerpv20250801preview.ApplicationGraphResponse,
	dependsOnInfo map[string][]*corerpv20250801preview.ApplicationGraphConnection,
) {
	if graph == nil || len(dependsOnInfo) == 0 {
		return
	}

	// Index by lower-cased ID; every lookup below normalizes the
	// caller-supplied ID the same way so mixed casing does not drop
	// edges.
	applicationGraphResourceByID := make(map[string]*corerpv20250801preview.ApplicationGraphResource, len(graph.Resources))
	for _, r := range graph.Resources {
		if r != nil && r.ID != nil {
			applicationGraphResourceByID[strings.ToLower(*r.ID)] = r
		}
	}
	if len(applicationGraphResourceByID) == 0 {
		return
	}

	for sourceID, entries := range dependsOnInfo {
		source := applicationGraphResourceByID[strings.ToLower(sourceID)]
		if source == nil {
			continue
		}
		if _, isExcluded := ExcludedResourceTypes[to.String(source.Type)]; isExcluded {
			continue
		}

		for _, entry := range entries {
			if entry == nil || entry.ID == nil || entry.Direction == nil || entry.Kind == nil {
				continue
			}
			if *entry.Direction != corerpv20250801preview.DirectionOutbound {
				continue
			}
			if *entry.Kind != corerpv20250801preview.ConnectionKindDependency {
				continue
			}

			target := applicationGraphResourceByID[strings.ToLower(*entry.ID)]
			if target == nil {
				continue
			}
			if _, isExcluded := ExcludedResourceTypes[to.String(target.Type)]; isExcluded {
				continue
			}

			// Skip if source already has any outbound edge to target.
			// This single check covers both "Connection wins over
			// Dependency" and "same Dependency pair sent twice".
			if hasOutbound(source.Connections, *target.ID) {
				continue
			}

			source.Connections = append(source.Connections, &corerpv20250801preview.ApplicationGraphConnection{
				ID:        to.Ptr(*target.ID),
				Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
				Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
			})
			target.Connections = append(target.Connections, &corerpv20250801preview.ApplicationGraphConnection{
				ID:        to.Ptr(*source.ID),
				Direction: to.Ptr(corerpv20250801preview.DirectionInbound),
				Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
			})
		}
	}

	// Re-sort every resource so the appended edges land in a stable
	// (Direction, Kind, ID) order. Cheap because typical graphs have
	// only a handful of connections per resource.
	for _, r := range graph.Resources {
		if r != nil {
			sortConnections(r.Connections)
		}
	}
}

// hasOutbound reports whether conns contains any outbound edge whose
// ID matches targetID case-insensitively.
func hasOutbound(conns []*corerpv20250801preview.ApplicationGraphConnection, targetID string) bool {
	for _, c := range conns {
		if c == nil || c.ID == nil || c.Direction == nil {
			continue
		}
		if *c.Direction != corerpv20250801preview.DirectionOutbound {
			continue
		}
		if strings.EqualFold(*c.ID, targetID) {
			return true
		}
	}
	return false
}

// sortConnections orders a Connections slice deterministically by
// (Direction, Kind, ID). Nil pointers are pushed to the end.
func sortConnections(conns []*corerpv20250801preview.ApplicationGraphConnection) {
	sort.Slice(conns, func(i, j int) bool {
		a, b := conns[i], conns[j]
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		if ad, bd := stringOrEmpty((*string)(a.Direction)), stringOrEmpty((*string)(b.Direction)); ad != bd {
			return ad < bd
		}
		if ak, bk := stringOrEmpty((*string)(a.Kind)), stringOrEmpty((*string)(b.Kind)); ak != bk {
			return ak < bk
		}
		return to.String(a.ID) < to.String(b.ID)
	})
}

func stringOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
