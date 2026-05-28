// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cytoscape

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/graph"
)

func TestSerializer_Format(t *testing.T) {
	t.Parallel()
	assert.Equal(t, FormatName, NewSerializer().Format())
}

func TestSerialize_NilGraph(t *testing.T) {
	t.Parallel()

	_, err := NewSerializer().Serialize(context.Background(), nil)
	assert.Error(t, err)
}

func TestSerialize_EmptyGraph(t *testing.T) {
	t.Parallel()

	p, err := NewSerializer().Serialize(context.Background(), &graph.Graph{})
	require.NoError(t, err)
	require.NotNil(t, p)

	var doc Document
	require.NoError(t, json.Unmarshal(p.Data, &doc))
	assert.Empty(t, doc.Elements)
	assert.Equal(t, "application/json", p.ContentType)
	assert.Equal(t, FormatName, p.Format)
}

func TestSerialize_NodesAndEdges(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		Nodes: []graph.Node{
			{ID: "a", Name: "Alpha", Type: "T", Properties: map[string]any{"extra": "x"}},
			{ID: "b", Name: "Beta", Type: "T"},
		},
		Edges: []graph.Edge{
			{Source: "a", Target: "b", Kind: "Outbound"}, // no ID → derived
			{ID: "e1", Source: "b", Target: "a", Kind: "Inbound", Properties: map[string]any{"weight": float64(2)}},
		},
	}

	p, err := NewSerializer().Serialize(context.Background(), g)
	require.NoError(t, err)

	var doc Document
	require.NoError(t, json.Unmarshal(p.Data, &doc))
	require.Len(t, doc.Elements, 4)

	// First two should be nodes, last two edges, in input order.
	assert.Equal(t, "nodes", doc.Elements[0].Group)
	assert.Equal(t, "a", doc.Elements[0].Data["id"])
	assert.Equal(t, "Alpha", doc.Elements[0].Data["label"])
	assert.Equal(t, "T", doc.Elements[0].Data["type"])
	assert.Equal(t, "x", doc.Elements[0].Data["extra"])

	assert.Equal(t, "nodes", doc.Elements[1].Group)
	assert.Equal(t, "b", doc.Elements[1].Data["id"])

	assert.Equal(t, "edges", doc.Elements[2].Group)
	assert.Equal(t, "e0", doc.Elements[2].Data["id"], "missing edge id should be derived as e<index>")
	assert.Equal(t, "a", doc.Elements[2].Data["source"])
	assert.Equal(t, "b", doc.Elements[2].Data["target"])
	assert.Equal(t, "Outbound", doc.Elements[2].Data["kind"])

	assert.Equal(t, "edges", doc.Elements[3].Group)
	assert.Equal(t, "e1", doc.Elements[3].Data["id"])
	assert.Equal(t, float64(2), doc.Elements[3].Data["weight"])
}

// TestSerialize_DemoApp exercises a realistic application graph (frontend
// container connected to a redis cache) and asserts the exact Cytoscape JSON
// output. It doubles as documentation of the wire format consumed by
// Cytoscape.js.
func TestSerialize_DemoApp(t *testing.T) {
	t.Parallel()

	g := &graph.Graph{
		ID:   "/planes/radius/local/resourceGroups/default/providers/Applications.Core/applications/demo",
		Name: "demo",
		Nodes: []graph.Node{
			{
				ID:   "frontend",
				Type: "Applications.Core/containers",
				Name: "frontend",
				Properties: map[string]any{
					"image": "nginx:1.25",
				},
			},
			{
				ID:   "db",
				Type: "Applications.Datastores/redisCaches",
				Name: "db",
			},
		},
		Edges: []graph.Edge{
			{
				ID:     "frontend->db",
				Source: "frontend",
				Target: "db",
				Kind:   "Outbound",
			},
			{
				// empty ID → auto-generated as "e1"
				Source: "db",
				Target: "frontend",
				Kind:   "Inbound",
			},
		},
	}

	const expected = `{
  "elements": [
    {
      "group": "nodes",
      "data": {
        "id": "frontend",
        "image": "nginx:1.25",
        "label": "frontend",
        "type": "Applications.Core/containers"
      }
    },
    {
      "group": "nodes",
      "data": {
        "id": "db",
        "label": "db",
        "type": "Applications.Datastores/redisCaches"
      }
    },
    {
      "group": "edges",
      "data": {
        "id": "frontend->db",
        "kind": "Outbound",
        "source": "frontend",
        "target": "db"
      }
    },
    {
      "group": "edges",
      "data": {
        "id": "e1",
        "kind": "Inbound",
        "source": "db",
        "target": "frontend"
      }
    }
  ]
}`

	p, err := NewSerializer().Serialize(context.Background(), g)
	require.NoError(t, err)
	assert.Equal(t, "application/json", p.ContentType)
	assert.Equal(t, FormatName, p.Format)
	assert.JSONEq(t, expected, string(p.Data))
}
