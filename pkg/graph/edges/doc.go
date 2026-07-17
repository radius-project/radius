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

// Package edges hosts the shared Dependency-edge merge helper used by
// both the CLI static-graph builder (`rad app graph <app.bicep>`) and
// the Radius.Core/2025-08-01-preview runtime handler (`getGraph`
// action).
//
// Both producers build a Connection-only graph from their own inputs
// (Bicep template on the CLI side, stored resource records on the
// server side) and then call MergeDependencyEdges to overlay a set of
// caller-supplied Dependency edges.
//
// The helper operates on the wire model
// (corerpv20250801preview.ApplicationGraphConnection) so there is one
// canonical edge type across the codebase.
//
// Layering: pkg/graph/edges -> pkg/corerp/api/v20250801preview. That is
// intentional. Anything that can import the wire model can import this
// package (CLI, server, tests). Server code MUST NOT import from
// pkg/cli/; the fact that this package sits under pkg/graph/ rather
// than pkg/cli/ keeps that layering clean.
package edges
