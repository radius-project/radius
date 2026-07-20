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

	byID := make(map[string]*corerpv20250801preview.ApplicationGraphResource, len(graph.Resources))
	for _, r := range graph.Resources {
		if r != nil && r.ID != nil {
			byID[*r.ID] = r
		}
	}
	if len(byID) == 0 {
		return
	}

	// (source, target) pairs that already have an outbound Connection
	// on source. Populated on demand as we walk incoming edges to
	// avoid touching every resource when incoming is small.
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
			connectionPairs[pair{sourceID, *c.ID}] = struct{}{}
		}
	}

	// De-dup incoming pairs before emitting anything, so the same
	// (source, target) sent twice by a sloppy caller becomes one edge.
	emitted := map[pair]struct{}{}
	touched := map[string]struct{}{}

	for sourceID, entries := range incoming {
		sourceRes, ok := byID[sourceID]
		if !ok {
			continue
		}
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
			targetID := *entry.ID
			targetRes, ok := byID[targetID]
			if !ok {
				continue
			}
			if _, isExcluded := excluded[to.String(targetRes.Type)]; isExcluded {
				continue
			}
			p := pair{sourceID, targetID}
			if _, wins := connectionPairs[p]; wins {
				continue // Connection wins over Dependency for the same pair.
			}
			if _, dup := emitted[p]; dup {
				continue
			}
			emitted[p] = struct{}{}

			sourceRes.Connections = append(sourceRes.Connections,
				&corerpv20250801preview.ApplicationGraphConnection{
					ID:        to.Ptr(targetID),
					Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
					Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
				})
			targetRes.Connections = append(targetRes.Connections,
				&corerpv20250801preview.ApplicationGraphConnection{
					ID:        to.Ptr(sourceID),
					Direction: to.Ptr(corerpv20250801preview.DirectionInbound),
					Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
				})
			touched[sourceID] = struct{}{}
			touched[targetID] = struct{}{}
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
