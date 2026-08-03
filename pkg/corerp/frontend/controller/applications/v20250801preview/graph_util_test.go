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

package v20250801preview

import (
	"testing"

	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The end-to-end filter policy (excluded types, Connection-wins, direction and
// kind validation, dedupe) is exhaustively covered in
// pkg/graph/edges/edges_test.go. The tests here only assert the server-side
// plumbing:
//
//  1. computeGraph forwards dependsOnEdges to edges.MergeDependencyEdges.
//  2. edges.ExcludedResourceTypes drops caller-supplied edges from application
//     and environment sources.
//  3. Missing / nil dependsOnEdges is a no-op.
func Test_computeGraph_MergesDependsOnEdges(t *testing.T) {
	const (
		containerID = "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers/consumer"
		queueID     = "/planes/radius/local/resourceGroups/default/providers/Radius.Messaging/rabbitMQQueues/queue"
		appID       = "/planes/radius/local/resourceGroups/default/providers/Radius.Core/applications/myapp"
	)

	container := generated.GenericResource{
		ID:   to.Ptr(containerID),
		Name: to.Ptr("consumer"),
		Type: to.Ptr("Radius.Compute/containers"),
		Properties: map[string]any{
			"application": appID,
		},
	}
	queue := generated.GenericResource{
		ID:   to.Ptr(queueID),
		Name: to.Ptr("queue"),
		Type: to.Ptr("Radius.Messaging/rabbitMQQueues"),
		Properties: map[string]any{
			"application": appID,
		},
	}
	appResource := generated.GenericResource{
		ID:   to.Ptr(appID),
		Name: to.Ptr("myapp"),
		Type: to.Ptr("Radius.Core/applications"),
	}

	dependsOnEdge := &corerpv20250801preview.ApplicationGraphConnection{
		ID:        to.Ptr(queueID),
		Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
		Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
	}

	findResource := func(t *testing.T, graph *corerpv20250801preview.ApplicationGraphResponse, id string) *corerpv20250801preview.ApplicationGraphResource {
		t.Helper()
		for _, r := range graph.Resources {
			if r != nil && r.ID != nil && *r.ID == id {
				return r
			}
		}
		require.Failf(t, "resource not in graph", "id=%s", id)
		return nil
	}

	t.Run("dependsOnEdges are merged", func(t *testing.T) {
		graph := computeGraph(
			[]generated.GenericResource{container, queue},
			nil,
			"",
			map[string][]*corerpv20250801preview.ApplicationGraphConnection{
				containerID: {dependsOnEdge},
			},
		)

		// Container gains an outbound Dependency edge to queue.
		containerNode := findResource(t, graph, containerID)
		var haveOutboundDep bool
		for _, c := range containerNode.Connections {
			if c == nil || c.ID == nil || c.Direction == nil || c.Kind == nil {
				continue
			}
			if *c.ID == queueID &&
				*c.Direction == corerpv20250801preview.DirectionOutbound &&
				*c.Kind == corerpv20250801preview.ConnectionKindDependency {
				haveOutboundDep = true
			}
		}
		assert.True(t, haveOutboundDep, "container should carry outbound Dependency edge to queue")

		// Queue gains a mirrored inbound Dependency edge from container.
		queueNode := findResource(t, graph, queueID)
		var haveInboundDep bool
		for _, c := range queueNode.Connections {
			if c == nil || c.ID == nil || c.Direction == nil || c.Kind == nil {
				continue
			}
			if *c.ID == containerID &&
				*c.Direction == corerpv20250801preview.DirectionInbound &&
				*c.Kind == corerpv20250801preview.ConnectionKindDependency {
				haveInboundDep = true
			}
		}
		assert.True(t, haveInboundDep, "queue should carry mirrored inbound Dependency edge from container")
	})

	t.Run("nil dependsOnEdges leaves graph unchanged", func(t *testing.T) {
		graph := computeGraph(
			[]generated.GenericResource{container, queue},
			nil,
			"",
			nil,
		)
		containerNode := findResource(t, graph, containerID)
		for _, c := range containerNode.Connections {
			if c != nil && c.Kind != nil {
				assert.NotEqual(t, corerpv20250801preview.ConnectionKindDependency, *c.Kind,
					"no Dependency edges should be present when dependsOnEdges is nil")
			}
		}
	})

	t.Run("edges sourced from excluded types are dropped", func(t *testing.T) {
		// Radius.Core/applications is in edges.ExcludedResourceTypes, so a
		// caller-supplied edge sourced from it must not land on the graph.
		graph := computeGraph(
			[]generated.GenericResource{container, queue, appResource},
			nil,
			"",
			map[string][]*corerpv20250801preview.ApplicationGraphConnection{
				appID: {{
					ID:        to.Ptr(queueID),
					Direction: to.Ptr(corerpv20250801preview.DirectionOutbound),
					Kind:      to.Ptr(corerpv20250801preview.ConnectionKindDependency),
				}},
			},
		)
		// Queue should not carry an inbound Dependency edge from appID.
		queueNode := findResource(t, graph, queueID)
		for _, c := range queueNode.Connections {
			if c == nil || c.ID == nil {
				continue
			}
			assert.NotEqual(t, appID, *c.ID, "excluded-source Dependency edge should not be merged")
		}
	})
}
