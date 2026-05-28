// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

// Package graph provides a backend-agnostic model for Radius application
// graphs along with pluggable serializers (e.g. Cytoscape) and persistence
// backends (e.g. git branch, graph database).
//
// The package is organized into three layers:
//
//   - model.go              : canonical in-memory Graph type and JSON I/O.
//   - serialize/...         : transforms the model into renderer-specific
//     formats consumed by visualization libraries.
//   - persistence/...       : pluggable Store implementations for saving and
//     loading serialized graphs.
//
// Callers typically compose the layers as:
//
//	g, err := graph.FromJSON(input)              // parse producer JSON
//	payload, err := s.Serialize(ctx, g)          // e.g. cytoscape.Serializer
//	err = store.Save(ctx, key, payload)          // e.g. git.Store
package graph
