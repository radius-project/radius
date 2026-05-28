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

package build

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/radius-project/radius/pkg/graph"
)

// ToGraph adapts a StaticGraphArtifact into the canonical pkg/graph.Graph
// model consumed by serializers (e.g. cytoscape) and persistence Stores that
// operate on graph.Graph rather than the build-specific types.
//
// Each Resource becomes a Node; each outbound Connection becomes an Edge.
// Inbound connections are intentionally skipped because they are reverse
// projections of outbound edges and would double-count.
func ToGraph(a *StaticGraphArtifact) *graph.Graph {
	if a == nil {
		return &graph.Graph{}
	}

	g := &graph.Graph{
		Metadata: map[string]string{
			"version":     a.Version,
			"generatedAt": a.GeneratedAt,
			"sourceFile":  a.SourceFile,
		},
		Nodes: make([]graph.Node, 0, len(a.Application.Resources)),
	}

	for _, r := range a.Application.Resources {
		props := map[string]any{
			"provisioningState": r.ProvisioningState,
		}
		if r.CodeReference != "" {
			props["codeReference"] = r.CodeReference
		}
		if r.AppDefinitionLine != 0 {
			props["appDefinitionLine"] = r.AppDefinitionLine
		}
		if r.DiffHash != "" {
			props["diffHash"] = r.DiffHash
		}

		g.Nodes = append(g.Nodes, graph.Node{
			ID:         r.ID,
			Type:       r.Type,
			Name:       r.Name,
			Properties: props,
		})

		for i, c := range r.Connections {
			if c.Direction != DirectionOutbound {
				continue
			}
			g.Edges = append(g.Edges, graph.Edge{
				ID:     r.ID + "->" + c.ID + "#" + strconv.Itoa(i),
				Source: r.ID,
				Target: c.ID,
				Kind:   DirectionOutbound,
			})
		}
	}

	return g
}

// Marshal returns the canonical JSON serialization of the artifact suitable
// for writing to a persistence.Store. Indented for human-friendly diffs on
// the orphan branch.
func Marshal(a *StaticGraphArtifact) ([]byte, error) {
	if a == nil {
		return nil, errors.New("build: nil artifact")
	}
	return json.MarshalIndent(a, "", "  ")
}
