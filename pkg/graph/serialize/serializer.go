// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package serialize defines the Serializer interface implemented by format
// adapters (e.g. cytoscape) that convert a graph.Graph into a payload
// consumable by a specific visualization library.
package serialize

import (
	"context"

	"github.com/radius-project/radius/pkg/graph"
)

// Payload is the serialized form of a Graph.
type Payload struct {
	// ContentType describes the MIME type of Data (e.g. "application/json").
	ContentType string

	// Format is a short identifier for the serialization format
	// (e.g. "cytoscape", "mermaid").
	Format string

	// Data is the serialized bytes.
	Data []byte
}

// Serializer converts a graph.Graph into a Payload for a specific consumer.
//
// Implementations must be safe for concurrent use.
type Serializer interface {
	// Format returns the short identifier for the serialization format.
	Format() string

	// Serialize converts the supplied graph into a Payload.
	Serialize(ctx context.Context, g *graph.Graph) (*Payload, error)
}
