// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Graph is the canonical, backend-agnostic representation of an application
// graph used by serializers and persistence stores in this package.
// It is close to what graph visual renderers (ex: cytoscape) expect.
type Graph struct {
	// ID uniquely identifies this graph (e.g. application resource ID).
	ID string `json:"id"`

	// Name is a human-readable name for the graph.
	Name string `json:"name"`

	// Nodes are the vertices of the graph.
	Nodes []Node `json:"nodes"`

	// Edges are the directed connections between nodes.
	Edges []Edge `json:"edges"`

	// Metadata carries free-form key/value information about the graph
	// (e.g. environment, application, generation timestamp).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Node represents a vertex in the Graph.
type Node struct {
	// ID uniquely identifies the node within the Graph.
	ID string `json:"id"`

	// Type is an optional type discriminator (e.g. a Radius resource type).
	Type string `json:"type,omitempty"`

	// Name is an optional human-readable label for the node.
	Name string `json:"name,omitempty"`

	// Properties carries arbitrary key/value data attached to the node.
	Properties map[string]any `json:"properties,omitempty"`

	// Labels are short string tags used for grouping or filtering.
	Labels map[string]string `json:"labels,omitempty"`
}

// Edge represents a directed connection between two Nodes.
type Edge struct {
	// ID is an optional stable identifier for the edge. Serializers may
	// derive one when empty.
	ID string `json:"id,omitempty"`

	// Source is the ID of the originating node.
	Source string `json:"source"`

	// Target is the ID of the destination node.
	Target string `json:"target"`

	// Kind is an optional edge category (e.g. "Outbound", "DependsOn").
	Kind string `json:"kind,omitempty"`

	// Properties carries arbitrary key/value data attached to the edge.
	Properties map[string]any `json:"properties,omitempty"`

	// Labels are short string tags used for grouping or filtering.
	Labels map[string]string `json:"labels,omitempty"`
}

// FromJSON decodes a JSON document into a Graph.
//
// The input is expected to already conform to the Graph schema. Producers
// that emit a different shape (e.g. corerp.ApplicationGraphResponse) should
// adapt to Graph before calling persistence/serialization APIs.
func FromJSON(data []byte) (*Graph, error) {
	if len(data) == 0 {
		return nil, errors.New("graph data is empty")
	}
	g := &Graph{}
	if err := json.Unmarshal(data, g); err != nil {
		return nil, fmt.Errorf("error while decoding graph data: %w", err)
	}
	return g, nil
}

// ToJSON encodes the Graph as a canonical JSON document.
func (g *Graph) ToJSON() ([]byte, error) {
	return json.MarshalIndent(g, "", "  ")
}
