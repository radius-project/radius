// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package cytoscape serializes a graph.Graph into the Cytoscape.js
// "elements" JSON format.
//
// See https://js.cytoscape.org/#notation/elements-json for the schema.
package cytoscape

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/radius-project/radius/pkg/graph"
	"github.com/radius-project/radius/pkg/graph/serialize"
)

// FormatName is the registered identifier for this serializer.
const FormatName = "cytoscape"

// Element is a single Cytoscape element (node or edge).
type Element struct {
	// Group is either "nodes" or "edges".
	Group string `json:"group"`

	// Data is the element's payload (id, source/target for edges, plus any
	// caller-supplied properties).
	Data map[string]any `json:"data"`
}

// Document is the top-level Cytoscape elements document.
type Document struct {
	// Elements are the nodes and edges of the document, in serialization
	// order (nodes first, then edges).
	Elements []Element `json:"elements"`
}

// Serializer implements serialize.Serializer for Cytoscape.js.
type Serializer struct{}

// NewSerializer returns a new Cytoscape serializer.
func NewSerializer() *Serializer { return &Serializer{} }

// Format returns the format identifier.
func (s *Serializer) Format() string { return FormatName }

// Serialize converts a graph.Graph into a Cytoscape elements JSON Payload.
func (s *Serializer) Serialize(_ context.Context, g *graph.Graph) (*serialize.Payload, error) {
	if g == nil {
		return nil, errors.New("error while serializing to cytoscape format: nil graph")
	}

	doc := Document{Elements: make([]Element, 0, len(g.Nodes)+len(g.Edges))}

	for _, n := range g.Nodes {
		data := map[string]any{"id": n.ID}
		if n.Name != "" {
			data["label"] = n.Name
		}
		if n.Type != "" {
			data["type"] = n.Type
		}
		for k, v := range n.Properties {
			data[k] = v
		}
		doc.Elements = append(doc.Elements, Element{Group: "nodes", Data: data})
	}

	for i, e := range g.Edges {
		id := e.ID
		if id == "" {
			id = fmt.Sprintf("e%d", i)
		}
		data := map[string]any{
			"id":     id,
			"source": e.Source,
			"target": e.Target,
		}
		if e.Kind != "" {
			data["kind"] = e.Kind
		}
		for k, v := range e.Properties {
			data[k] = v
		}
		doc.Elements = append(doc.Elements, Element{Group: "edges", Data: data})
	}

	bytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("cytoscape: encode: %w", err)
	}

	return &serialize.Payload{
		ContentType: "application/json",
		Format:      FormatName,
		Data:        bytes,
	}, nil
}
