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

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/require"
)

const (
	consumerID  = "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers/consumer"
	rabbitmqID  = "/planes/radius/local/resourcegroups/default/providers/Radius.Messaging/rabbitMQ/rabbitmq"
	appsecretID = "/planes/radius/local/resourcegroups/default/providers/Radius.Security/secrets/appsecret"
	appScopeID  = "/planes/radius/local/resourcegroups/default/providers/Radius.Core/applications/rabbitmq-app"

	containerType = "Radius.Compute/containers"
	queueType     = "Radius.Messaging/rabbitMQ"
	secretType    = "Radius.Security/secrets"
	appScopeType  = "Radius.Core/applications"
)

// excluded returns the standard exclusion set used by both the CLI
// static builder and the Radius.Core preview runtime handler.
func excluded() map[string]struct{} {
	return map[string]struct{}{
		appScopeType:                     {},
		"Radius.Core/environments":       {},
		"Radius.Core/recipePacks":        {},
		"Applications.Core/applications": {},
		"Applications.Core/environments": {},
	}
}

// resource is a small test helper that builds an ApplicationGraphResource
// with the given ID, Type, and pre-populated Connections.
func resource(id, typ string, conns ...*corerpv20250801preview.ApplicationGraphConnection) *corerpv20250801preview.ApplicationGraphResource {
	if conns == nil {
		conns = []*corerpv20250801preview.ApplicationGraphConnection{}
	}
	return &corerpv20250801preview.ApplicationGraphResource{
		ID:          to.Ptr(id),
		Type:        to.Ptr(typ),
		Connections: conns,
	}
}

// dep constructs a caller-supplied outbound Dependency entry pointing at
// the given target. Matches what the wire schema requires.
func dep(target string) *corerpv20250801preview.ApplicationGraphConnection {
	return &corerpv20250801preview.ApplicationGraphConnection{
		ID:        to.Ptr(target),
		Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
		Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
	}
}

// connOut constructs an outbound Kind: Connection edge, useful for
// seeding the graph before merging Dependency edges (Connection wins
// tests).
func connOut(target string) *corerpv20250801preview.ApplicationGraphConnection {
	return &corerpv20250801preview.ApplicationGraphConnection{
		ID:        to.Ptr(target),
		Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
		Kind:      to.Ptr(corerpv20250801preview.ConnectionKindConnection),
	}
}

func findResource(t *testing.T, graph *corerpv20250801preview.ApplicationGraphResponse, id string) *corerpv20250801preview.ApplicationGraphResource {
	t.Helper()
	for _, r := range graph.Resources {
		if r != nil && r.ID != nil && *r.ID == id {
			return r
		}
	}
	t.Fatalf("resource %s not in graph", id)
	return nil
}

func TestMergeDependencyEdges_NilGraphOrEmptyInputIsNoOp(t *testing.T) {
	t.Parallel()

	// Nil graph — no panic.
	MergeDependencyEdges(nil, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {dep(rabbitmqID)},
	}, excluded())

	// Empty incoming — no changes.
	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType),
			resource(rabbitmqID, queueType),
		},
	}
	MergeDependencyEdges(graph, nil, excluded())
	require.Empty(t, findResource(t, graph, consumerID).Connections)
	require.Empty(t, findResource(t, graph, rabbitmqID).Connections)
}

func TestMergeDependencyEdges_HappyPath(t *testing.T) {
	t.Parallel()

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType),
			resource(rabbitmqID, queueType),
			resource(appsecretID, secretType),
		},
	}
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {dep(rabbitmqID), dep(appsecretID)},
	}, excluded())

	// consumer has two outbound Dependency edges (sorted by target).
	consumer := findResource(t, graph, consumerID)
	require.Len(t, consumer.Connections, 2)
	for _, c := range consumer.Connections {
		require.Equal(t, corerpv20250801preview.DirectionOutbound, *c.Direction)
		require.Equal(t, corerpv20250801preview.ConnectionKindDependency, *c.Kind)
	}
	// Deterministic sort by ID: rabbitmq before appsecret because
	// "Radius.Messaging" sorts before "Radius.Security" lexically.
	require.Equal(t, rabbitmqID, *consumer.Connections[0].ID)
	require.Equal(t, appsecretID, *consumer.Connections[1].ID)

	// Each target has one inbound Dependency mirror.
	for _, targetID := range []string{rabbitmqID, appsecretID} {
		target := findResource(t, graph, targetID)
		require.Len(t, target.Connections, 1)
		require.Equal(t, corerpv20250801preview.DirectionInbound, *target.Connections[0].Direction)
		require.Equal(t, corerpv20250801preview.ConnectionKindDependency, *target.Connections[0].Kind)
		require.Equal(t, consumerID, *target.Connections[0].ID)
	}
}

func TestMergeDependencyEdges_ExcludedSourceDropsAllOutgoing(t *testing.T) {
	t.Parallel()

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(appScopeID, appScopeType),
			resource(rabbitmqID, queueType),
		},
	}
	// Caller nonsensically claims the app scope depends on rabbitmq.
	// The excluded-source rule silently drops it.
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		appScopeID: {dep(rabbitmqID)},
	}, excluded())
	require.Empty(t, findResource(t, graph, appScopeID).Connections)
	require.Empty(t, findResource(t, graph, rabbitmqID).Connections)
}

func TestMergeDependencyEdges_ExcludedTargetDropsEdge(t *testing.T) {
	t.Parallel()

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType),
			resource(appScopeID, appScopeType),
			resource(rabbitmqID, queueType),
		},
	}
	// consumer -> app (excluded) is dropped, consumer -> rabbitmq is
	// kept. Common shape in real Bicep templates.
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {dep(appScopeID), dep(rabbitmqID)},
	}, excluded())
	consumer := findResource(t, graph, consumerID)
	require.Len(t, consumer.Connections, 1)
	require.Equal(t, rabbitmqID, *consumer.Connections[0].ID)
	require.Empty(t, findResource(t, graph, appScopeID).Connections,
		"excluded target must not receive a mirrored inbound entry")
}

func TestMergeDependencyEdges_UnknownEndpointIsDropped(t *testing.T) {
	t.Parallel()

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType),
			resource(rabbitmqID, queueType),
		},
	}
	// Source not in graph.
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		"/planes/radius/local/resourcegroups/default/providers/Nowhere/things/x": {dep(rabbitmqID)},
	}, excluded())
	require.Empty(t, findResource(t, graph, consumerID).Connections)
	require.Empty(t, findResource(t, graph, rabbitmqID).Connections)

	// Target not in graph.
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {dep("/planes/radius/local/resourcegroups/default/providers/Nowhere/things/x")},
	}, excluded())
	require.Empty(t, findResource(t, graph, consumerID).Connections)
}

func TestMergeDependencyEdges_ConnectionWinsOverDependency(t *testing.T) {
	t.Parallel()

	// The graph already has a Kind: Connection outbound from consumer
	// to rabbitmq (mirrored inbound on rabbitmq). A caller-supplied
	// Dependency edge for the same pair MUST be dropped.
	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType, connOut(rabbitmqID)),
			resource(rabbitmqID, queueType, &corerpv20250801preview.ApplicationGraphConnection{
				ID:        to.Ptr(consumerID),
				Direction: to.Ptr(corerpv20250801preview.DirectionInbound),
				Kind:      to.Ptr(corerpv20250801preview.ConnectionKindConnection),
			}),
		},
	}
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {dep(rabbitmqID)},
	}, excluded())

	// consumer still has exactly one outbound entry, tagged Connection.
	consumer := findResource(t, graph, consumerID)
	require.Len(t, consumer.Connections, 1)
	require.Equal(t, corerpv20250801preview.ConnectionKindConnection, *consumer.Connections[0].Kind)

	// rabbitmq still has exactly one inbound entry, tagged Connection.
	rabbitmq := findResource(t, graph, rabbitmqID)
	require.Len(t, rabbitmq.Connections, 1)
	require.Equal(t, corerpv20250801preview.ConnectionKindConnection, *rabbitmq.Connections[0].Kind)
}

func TestMergeDependencyEdges_DuplicateIncomingPairsCollapse(t *testing.T) {
	t.Parallel()

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType),
			resource(rabbitmqID, queueType),
		},
	}
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {dep(rabbitmqID), dep(rabbitmqID), dep(rabbitmqID)},
	}, excluded())

	require.Len(t, findResource(t, graph, consumerID).Connections, 1,
		"repeated (source, target) pair in the same batch must collapse")
	require.Len(t, findResource(t, graph, rabbitmqID).Connections, 1)
}

func TestMergeDependencyEdges_MalformedInputEntriesAreSkipped(t *testing.T) {
	t.Parallel()

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			resource(consumerID, containerType),
			resource(rabbitmqID, queueType),
		},
	}
	MergeDependencyEdges(graph, map[string][]*corerpv20250801preview.ApplicationGraphConnection{
		consumerID: {
			nil, // nil entry
			{ID: to.Ptr(rabbitmqID)}, // missing Direction and Kind
			{ID: to.Ptr(rabbitmqID), Direction: to.Ptr(corerpv20250801preview.DirectionInbound), Kind: to.Ptr(corerpv20250801preview.ConnectionKindDependency)},  // wrong Direction
			{ID: to.Ptr(rabbitmqID), Direction: to.Ptr(corerpv20250801preview.DirectionOutbound), Kind: to.Ptr(corerpv20250801preview.ConnectionKindConnection)}, // wrong Kind
			dep(rabbitmqID), // valid — this one is emitted
		},
	}, excluded())

	consumer := findResource(t, graph, consumerID)
	require.Len(t, consumer.Connections, 1, "only the valid entry must be emitted")
	require.Equal(t, corerpv20250801preview.ConnectionKindDependency, *consumer.Connections[0].Kind)
}
