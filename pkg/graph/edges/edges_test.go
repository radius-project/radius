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
	"testing"

	"github.com/stretchr/testify/require"
)

// Test IDs. Short strings; the extractor is agnostic to their shape as
// long as they compare equal.
const (
	containerAID    = "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers/a"
	containerBID    = "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers/b"
	queueID         = "/planes/radius/local/resourcegroups/default/providers/Radius.Messaging/rabbitMQ/q"
	appScopeID      = "/planes/radius/local/resourcegroups/default/providers/Radius.Core/applications/scope"
	unknownID       = "/planes/radius/local/resourcegroups/default/providers/Unknown.Ns/things/x"
	containerType   = "Radius.Compute/containers"
	queueType       = "Radius.Messaging/rabbitMQ"
	appScopeType    = "Radius.Core/applications"
	radiusCoreScope = "Radius.Core/applications"
)

// TestExtractEdges is the exhaustive table-driven test for the extractor.
// Each case sets up a small resource graph, calls ExtractEdges, and
// asserts on the exact returned edge slice (already sorted
// deterministically by (Source, Target, Direction, Kind)).
func TestExtractEdges(t *testing.T) {
	t.Parallel()

	excluded := map[string]struct{}{
		radiusCoreScope: {},
	}

	tests := []struct {
		name      string
		resources []Resource
		excluded  map[string]struct{}
		want      []Edge
	}{
		{
			name:      "empty input produces empty output",
			resources: nil,
			excluded:  excluded,
			want:      []Edge{},
		},
		{
			name: "single Connection edge produces outbound plus mirrored inbound",
			resources: []Resource{
				{
					ID:          containerAID,
					Type:        containerType,
					Connections: []string{queueID},
				},
				{ID: queueID, Type: queueType},
			},
			excluded: excluded,
			want: []Edge{
				{Source: containerAID, Target: queueID, Direction: DirectionInbound, Kind: KindConnection},
				{Source: containerAID, Target: queueID, Direction: DirectionOutbound, Kind: KindConnection},
			},
		},
		{
			name: "single Dependency edge produces outbound plus mirrored inbound",
			resources: []Resource{
				{
					ID:        containerAID,
					Type:      containerType,
					DependsOn: []string{queueID},
				},
				{ID: queueID, Type: queueType},
			},
			excluded: excluded,
			want: []Edge{
				{Source: containerAID, Target: queueID, Direction: DirectionInbound, Kind: KindDependency},
				{Source: containerAID, Target: queueID, Direction: DirectionOutbound, Kind: KindDependency},
			},
		},
		{
			name: "same pair from both sources: Connection wins over Dependency",
			resources: []Resource{
				{
					ID:          containerAID,
					Type:        containerType,
					Connections: []string{queueID},
					DependsOn:   []string{queueID},
				},
				{ID: queueID, Type: queueType},
			},
			excluded: excluded,
			want: []Edge{
				{Source: containerAID, Target: queueID, Direction: DirectionInbound, Kind: KindConnection},
				{Source: containerAID, Target: queueID, Direction: DirectionOutbound, Kind: KindConnection},
			},
		},
		{
			name: "multiple dependsOn tokens to same target collapse to one edge",
			resources: []Resource{
				{
					ID:        containerAID,
					Type:      containerType,
					DependsOn: []string{queueID, queueID, queueID},
				},
				{ID: queueID, Type: queueType},
			},
			excluded: excluded,
			want: []Edge{
				{Source: containerAID, Target: queueID, Direction: DirectionInbound, Kind: KindDependency},
				{Source: containerAID, Target: queueID, Direction: DirectionOutbound, Kind: KindDependency},
			},
		},
		{
			name: "fan-in with mixed kinds preserves each source's kind on the target",
			resources: []Resource{
				{
					ID:          containerAID,
					Type:        containerType,
					Connections: []string{queueID},
				},
				{
					ID:        containerBID,
					Type:      containerType,
					DependsOn: []string{queueID},
				},
				{ID: queueID, Type: queueType},
			},
			excluded: excluded,
			want: []Edge{
				{Source: containerAID, Target: queueID, Direction: DirectionInbound, Kind: KindConnection},
				{Source: containerAID, Target: queueID, Direction: DirectionOutbound, Kind: KindConnection},
				{Source: containerBID, Target: queueID, Direction: DirectionInbound, Kind: KindDependency},
				{Source: containerBID, Target: queueID, Direction: DirectionOutbound, Kind: KindDependency},
			},
		},
		{
			name: "edge target of excluded type is dropped (no node, no edge, no mirror)",
			resources: []Resource{
				{
					ID:        containerAID,
					Type:      containerType,
					DependsOn: []string{appScopeID},
				},
				// Excluded resource is present in the input list.
				// The extractor must not treat it as a valid target.
				{ID: appScopeID, Type: appScopeType},
			},
			excluded: excluded,
			want:     []Edge{},
		},
		{
			name: "edge target not in the resource set is dropped",
			resources: []Resource{
				{
					ID:        containerAID,
					Type:      containerType,
					DependsOn: []string{unknownID},
				},
			},
			excluded: excluded,
			want:     []Edge{},
		},
		{
			name: "excluded source emits no edges even if it has connections",
			resources: []Resource{
				{
					ID:          appScopeID,
					Type:        appScopeType,
					Connections: []string{queueID},
				},
				{ID: queueID, Type: queueType},
			},
			excluded: excluded,
			want:     []Edge{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractEdges(tc.resources, tc.excluded)
			if got == nil {
				got = []Edge{}
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// TestExtractEdges_OwnerPeer verifies the Edge.Owner / Edge.Peer helpers
// used by callers when materializing wire-model entries.
func TestExtractEdges_OwnerPeer(t *testing.T) {
	t.Parallel()

	out := Edge{Source: "a", Target: "b", Direction: DirectionOutbound, Kind: KindConnection}
	require.Equal(t, "a", out.Owner())
	require.Equal(t, "b", out.Peer())

	in := Edge{Source: "a", Target: "b", Direction: DirectionInbound, Kind: KindConnection}
	require.Equal(t, "b", in.Owner())
	require.Equal(t, "a", in.Peer())
}

// TestExtractEdges_SortDeterminism confirms the returned edge slice is
// deterministic across input orderings: shuffling the input resources
// must not change the output.
func TestExtractEdges_SortDeterminism(t *testing.T) {
	t.Parallel()

	base := []Resource{
		{ID: containerAID, Type: containerType, DependsOn: []string{queueID}},
		{ID: containerBID, Type: containerType, DependsOn: []string{queueID}},
		{ID: queueID, Type: queueType},
	}
	shuffled := []Resource{base[2], base[0], base[1]}

	require.Equal(t,
		ExtractEdges(base, nil),
		ExtractEdges(shuffled, nil),
	)
}
