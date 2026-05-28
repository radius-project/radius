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

// Package build constructs a static application graph artifact from a
// compiled ARM JSON template and its original Bicep source.
//
// The build output (StaticGraphArtifact) is intended for two consumers:
//
//   - Persistence: it is JSON-marshaled and written to a persistence.Store
//     (typically the git orphan-branch Store under pkg/graph/persistence/git).
//   - Visualization: it can be adapted to the canonical pkg/graph.Graph type
//     via ToGraph, then handed to a serializer such as
//     pkg/graph/serialize/cytoscape.
//
// This package replaces the github-demo branch's pkg/cli/graph/{build,diffhash}.go
// and intentionally does not depend on pkg/corerp generated types so that the
// extra resource fields (codeReference, appDefinitionLine, diffHash) can be
// carried without modifying generated API models.
package build
