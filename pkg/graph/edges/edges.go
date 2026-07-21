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

// MergeDependencyEdges overlays a set of caller-supplied Dependency
// edges onto an already-built ApplicationGraphResponse. The graph is
// assumed to carry only Kind: Connection edges when this function is
// called; on return, each accepted incoming edge is present as
// Direction: Outbound on the source resource and mirrored as
// Direction: Inbound on the target, both tagged Kind: Dependency.
//
// The incoming map is keyed by source resource ID; the value is the
// list of outbound edges leaving that source. This matches the wire
// shape of GetGraphRequest.dependsOnEdges on
// Radius.Core/2025-08-01-preview, so the server can pass the request
// input directly. The CLI static-graph builder constructs an equivalent
// map from Bicep's compiled dependsOn list.
//
// ID matching is case-insensitive: Radius/ARM resource IDs are
// canonically case-insensitive, but casing varies in practice (for
// example server-returned IDs use "resourcegroups" while
// cli.RequireScope produces "resourceGroups"). Emitted edges reference
// the canonical ID form carried on graph.Resources so downstream
// consumers can look up either endpoint by exact-string match against
// the resources list.
//
// Filtering rules applied to every incoming edge:
//
//   - The edge is dropped if source or target is not present in
//     graph.Resources (unknown IDs are never rendered).
//   - The edge is dropped if source or target has a Type present in
//     excluded (control-plane containment types are never edge
//     endpoints).
//   - The edge is dropped if the (source, target) pair already has a
//     Kind: Connection outbound entry on source (Connection wins).
//   - Incoming entries whose Direction is not Outbound or whose Kind is
//     not Dependency are ignored — the server refuses
//     to trust caller-supplied Connection entries.
//   - The same (source, target) pair appearing multiple times in the
//     input collapses to a single edge (dependsOn de-duplication).
//
// After merging, each resource's Connections slice is sorted
// deterministically by (Direction, Kind, ID) so goldens and diffs
// stay stable.
//
// MergeDependencyEdges is a no-op when graph is nil, when incoming is
// empty, or when every incoming edge is filtered out.
func MergeDependencyEdges(
	graph *corerpv20250801preview.ApplicationGraphResponse,
	incoming map[string][]*corerpv20250801preview.ApplicationGraphConnection,
	excluded map[string]struct{},
) {
	if graph == nil || len(incoming) == 0 {
		return
	}

	// canonicalID resolves any casing of an incoming ID to the exact
	// string form that appears on graph.Resources. Populated once from
	// the resources list; all lookups below go through this map so
	// case-only differences (resourceGroups vs resourcegroups, mixed
	// provider casing, etc.) do not silently drop edges.
	canonicalID := make(map[string]string, len(graph.Resources))
	byID := make(map[string]*corerpv20250801preview.ApplicationGraphResource, len(graph.Resources))
	for _, r := range graph.Resources {
		if r != nil && r.ID != nil {
			byID[*r.ID] = r
			canonicalID[strings.ToLower(*r.ID)] = *r.ID
		}
	}
	if len(byID) == 0 {
		return
	}

	// (source, target) pairs that already have an outbound Connection
	// on source. Keyed by canonical IDs so the "Connection wins" check
	// works regardless of the casing the incoming edge uses.
	type pair struct{ src, tgt string }
	connectionPairs := map[pair]struct{}{}
	for sourceID, res := range byID {
		for _, c := range res.Connections {
			if c == nil || c.ID == nil || c.Direction == nil {
				continue
			}
			if *c.Direction != corerpv20250801preview.DirectionOutbound {
				continue
			}
			targetID, ok := canonicalID[strings.ToLower(*c.ID)]
			if !ok {
				// Existing Connection points outside the resources
				// list; keep the pair keyed by the raw target so a
				// future incoming Dependency with the same raw target
				// still loses to it.
				targetID = *c.ID
			}
			connectionPairs[pair{sourceID, targetID}] = struct{}{}
		}
	}

	// De-dup incoming pairs before emitting anything, so the same
	// (source, target) sent twice by a sloppy caller becomes one edge.
	emitted := map[pair]struct{}{}
	touched := map[string]struct{}{}

	for sourceID, entries := range incoming {
		canonicalSourceID, ok := canonicalID[strings.ToLower(sourceID)]
		if !ok {
			continue
		}
		sourceRes := byID[canonicalSourceID]
		if _, isExcluded := excluded[to.String(sourceRes.Type)]; isExcluded {
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
			canonicalTargetID, ok := canonicalID[strings.ToLower(*entry.ID)]
			if !ok {
				continue
			}
			targetRes := byID[canonicalTargetID]
			if _, isExcluded := excluded[to.String(targetRes.Type)]; isExcluded {
				continue
			}
			p := pair{canonicalSourceID, canonicalTargetID}
			if _, wins := connectionPairs[p]; wins {
				continue // Connection wins over Dependency for the same pair.
			}
			if _, dup := emitted[p]; dup {
				continue
			}
			emitted[p] = struct{}{}

			sourceRes.Connections = append(sourceRes.Connections,
				&corerpv20250801preview.ApplicationGraphConnection{
					ID:        to.Ptr(canonicalTargetID),
					Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
					Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
				})
			targetRes.Connections = append(targetRes.Connections,
				&corerpv20250801preview.ApplicationGraphConnection{
					ID:        to.Ptr(canonicalSourceID),
					Direction: to.Ptr(corerpv20250801preview.DirectionInbound),
					Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
				})
			touched[canonicalSourceID] = struct{}{}
			touched[canonicalTargetID] = struct{}{}
		}
	}

	// Sort every touched resource's Connections deterministically so
	// downstream diff/hash consumers see a stable order.
	for id := range touched {
		res := byID[id]
		sortConnections(res.Connections)
	}
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
