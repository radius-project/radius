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

import "sort"

// Edge kind constants. Values match the ConnectionKind enum on the
// Radius.Core/2025-08-01-preview wire model; keeping them as untyped
// strings here avoids importing the generated API package into this
// otherwise dependency-free package. Callers convert Edge.Kind into the
// generated ConnectionKind pointer when materializing the wire model.
const (
	// KindConnection tags edges derived from a resource's
	// properties.connections block (author-declared).
	KindConnection = "Connection"

	// KindDependency tags edges derived from a resource's dependsOn
	// list (implicit, inferred from Bicep's compiled template or the
	// caller-supplied dependsOnEdges on the runtime GetGraphRequest
	// wire).
	KindDependency = "Dependency"
)

// Edge direction constants. Values match the Direction enum on the
// Radius.Core/2025-08-01-preview wire model.
const (
	// DirectionOutbound identifies the source resource's view of an
	// edge: "I depend on / connect to Target".
	DirectionOutbound = "Outbound"

	// DirectionInbound identifies the target resource's view of the
	// same edge: "Source depends on / connects to me". Every outbound
	// edge is mirrored by an inbound edge on the target with the same
	// Kind.
	DirectionInbound = "Inbound"
)

// Resource is a graph-eligible Radius resource as seen by the edge
// extractor. Callers convert their own representation (ARM JSON entry,
// stored resource record, etc.) into this shape before calling
// ExtractEdges.
//
// Both Connections and DependsOn hold pre-resolved canonical Radius
// resource IDs. Callers own the resolution: the static caller parses
// ARM "[resourceId('T','N')]" expressions before calling the
// extractor; the runtime caller (Phase 2) receives already-resolved
// IDs from stored properties and from caller-supplied dependsOnEdges
// on the GetGraphRequest wire. Keeping the primitive free of ARM /
// storage / HTTP concerns is what makes it callable from both
// contexts.
type Resource struct {
	// ID is the canonical Radius resource ID
	// ("/planes/radius/local/resourcegroups/…/providers/{ns}/{type}/{name}").
	ID string

	// Type is the Radius resource type ("Radius.Compute/containers").
	Type string

	// Connections is the list of canonical Radius resource IDs this
	// resource author-declared under properties.connections[*].source.
	// Each entry produces a Kind=Connection edge (subject to exclusion
	// and de-duplication). Duplicates are collapsed silently.
	Connections []string

	// DependsOn is the list of canonical Radius resource IDs the
	// resource declares as build-time dependencies. Each entry not
	// already covered by Connections produces a Kind=Dependency edge.
	// The static caller populates this from resolveDependsOn on the
	// ARM template's dependsOn array. The runtime caller populates it
	// from caller-supplied dependsOnEdges on the GetGraphRequest wire.
	DependsOn []string
}

// Edge is a single directed edge in the application graph.
//
// Source and Target describe the fixed orientation of the underlying
// edge concept ("Source depends on / connects to Target"). Direction
// selects which side of that edge this entry describes: an Outbound
// entry lives on Source's Connections slice; an Inbound entry lives on
// Target's Connections slice.
//
// Use Owner and Peer to translate an Edge into a wire-model
// ApplicationGraphConnection entry without having to check Direction
// yourself.
type Edge struct {
	// Source is the canonical resource ID of the edge's source node
	// (the resource that depends on or connects to Target).
	Source string

	// Target is the canonical resource ID of the edge's target node.
	Target string

	// Direction is DirectionOutbound or DirectionInbound. Every
	// outbound edge Source→Target is mirrored by an inbound edge on
	// Target with the same Kind.
	Direction string

	// Kind is KindConnection (from properties.connections) or
	// KindDependency (from DependsOn). Case-sensitive; matches the
	// wire enum values on Radius.Core/2025-08-01-preview.
	Kind string
}

// Owner returns the canonical ID of the resource whose Connections
// slice owns this Edge entry: Source for an Outbound edge, Target for
// an Inbound edge.
func (e Edge) Owner() string {
	if e.Direction == DirectionInbound {
		return e.Target
	}
	return e.Source
}

// Peer returns the canonical ID of the other end of the edge — the
// value stored in the wire entry's `id` field on the owning resource's
// Connections slice.
func (e Edge) Peer() string {
	if e.Direction == DirectionInbound {
		return e.Source
	}
	return e.Target
}

// ExtractEdges returns the deduplicated, mirrored edge list for the
// given resources, dropping any edge whose source or target type is
// present in the excluded set.
//
// excluded holds canonical "Namespace/type" strings that MUST NOT
// appear as graph nodes or edge targets. Callers control membership so
// the same primitive can be used with different exclusion sets (static
// vs runtime).
//
// The output is sorted deterministically by (Source, Target,
// Direction, Kind) so callers can compare or diff it.
//
// ExtractEdges never returns an error: unresolvable DependsOn entries
// and edges targeting excluded types are silently dropped. See the
// spec's edge-cases section for the exhaustive rules.
//
// # Callers
//
// Two callers consume this primitive today, both by converting their
// own input into []Resource and interpreting the returned []Edge via
// Edge.Owner / Edge.Peer when materializing wire entries:
//
//   - Static graph builder — [pkg/cli/graph.BuildModeledGraph]. Resolves
//     ARM "[resourceId('T','N')]" expressions in
//     properties.connections and dependsOn before calling ExtractEdges.
//   - Runtime graph handler (Phase 2) —
//     pkg/corerp/frontend/controller/applications/v20250801preview.
//     Populates Resource.DependsOn from the caller-supplied
//     dependsOnEdges field on the GetGraphRequest wire (NOT from a
//     server-side property scan). Resource.Connections comes from the
//     resource's stored properties.connections[*].source, which is
//     already a canonical Radius resource ID at that point. Everything
//     else — exclusion, Connection-wins de-dup, mirroring — is reused
//     verbatim.
//
// The package intentionally imports nothing from pkg/cli/, pkg/corerp/,
// or net/http. Verified with `go list -deps ./pkg/graph/edges/...`.
// Keeping the primitive dependency-free is what makes Phase 2 a wiring
// change rather than a re-implementation (FR-017..FR-019).
//
// Future extensibility: if a second configuration knob becomes
// necessary (for example, an option to emit diagnostic reasons for
// dropped edges), promote both arguments into an ExtractOptions
// struct. Keeping a single positional argument today (Constitution
// VII, Simplicity Over Cleverness) avoids paying that cost until a
// real need materializes.
func ExtractEdges(resources []Resource, excluded map[string]struct{}) []Edge {
	// Build a canonical-ID → Type map for O(1) target validation.
	// Every candidate target must resolve to a Resource in this map;
	// unresolved targets and targets whose type is excluded are
	// silently dropped.
	byID := make(map[string]string, len(resources))
	for i := range resources {
		byID[resources[i].ID] = resources[i].Type
	}

	validTarget := func(id string) bool {
		typ, ok := byID[id]
		if !ok {
			return false
		}
		_, isExcluded := excluded[typ]
		return !isExcluded
	}

	// Collect outbound edges keyed by (source, target) → Kind so that
	// a Connection recorded first is never downgraded to a Dependency
	// by a same-pair dependsOn entry (Connection-wins, FR-011). Also
	// collapses multiple dependsOn tokens targeting the same resource
	// to one edge (FR-012).
	type pair struct{ src, tgt string }
	outbound := make(map[pair]string)

	for _, r := range resources {
		if _, isExcluded := excluded[r.Type]; isExcluded {
			continue // excluded types are never sources
		}

		// Connection edges from author-declared Connections.
		for _, target := range r.Connections {
			if !validTarget(target) {
				continue
			}
			// Connection wins: overwrite any pre-existing entry (there
			// shouldn't be one from Connection, but this makes the
			// invariant explicit).
			outbound[pair{r.ID, target}] = KindConnection
		}

		// Dependency edges from DependsOn. Skip if the pair is already
		// a Connection (Connection-wins). Collapse duplicate tokens.
		for _, dep := range r.DependsOn {
			if !validTarget(dep) {
				continue
			}
			k := pair{r.ID, dep}
			if _, exists := outbound[k]; exists {
				continue
			}
			outbound[k] = KindDependency
		}
	}

	// Emit outbound edges + mirrored inbound edges. Mirroring preserves
	// Kind per FR-010.
	out := make([]Edge, 0, len(outbound)*2)
	for p, kind := range outbound {
		out = append(out, Edge{
			Source:    p.src,
			Target:    p.tgt,
			Direction: DirectionOutbound,
			Kind:      kind,
		})
		out = append(out, Edge{
			Source:    p.src,
			Target:    p.tgt,
			Direction: DirectionInbound,
			Kind:      kind,
		})
	}

	// Deterministic sort so callers can compare or diff the output.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		if out[i].Target != out[j].Target {
			return out[i].Target < out[j].Target
		}
		if out[i].Direction != out[j].Direction {
			return out[i].Direction < out[j].Direction
		}
		return out[i].Kind < out[j].Kind
	})

	return out
}
