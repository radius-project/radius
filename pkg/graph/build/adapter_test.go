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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToGraph_NilArtifact(t *testing.T) {
	t.Parallel()

	g := ToGraph(nil)
	require.NotNil(t, g)
	assert.Empty(t, g.Nodes)
	assert.Empty(t, g.Edges)
}

func TestToGraph_MapsResourcesAndOutboundEdges(t *testing.T) {
	t.Parallel()

	art := &StaticGraphArtifact{
		Version:     "1.0.0",
		GeneratedAt: "2026-05-27T00:00:00Z",
		SourceFile:  "app.bicep",
		Application: Application{
			Resources: []*Resource{
				{
					ID:                "/planes/radius/local/.../containers/frontend",
					Type:              "Applications.Core/containers",
					Name:              "frontend",
					ProvisioningState: "Succeeded",
					CodeReference:     "src/frontend.ts#L1",
					AppDefinitionLine: 3,
					DiffHash:          "sha256:abc",
					Connections: []*Connection{
						{ID: "/planes/radius/local/.../containers/backend", Direction: DirectionOutbound},
						{ID: "/planes/radius/local/.../applications/myapp", Direction: DirectionInbound},
					},
				},
				{
					ID:                "/planes/radius/local/.../containers/backend",
					Type:              "Applications.Core/containers",
					Name:              "backend",
					ProvisioningState: "Succeeded",
				},
			},
		},
	}

	g := ToGraph(art)

	require.Len(t, g.Nodes, 2)
	require.Len(t, g.Edges, 1, "only the outbound connection should become an edge")

	assert.Equal(t, "Outbound", g.Edges[0].Kind)
	assert.Equal(t, "/planes/radius/local/.../containers/frontend", g.Edges[0].Source)
	assert.Equal(t, "/planes/radius/local/.../containers/backend", g.Edges[0].Target)

	// Metadata is populated.
	assert.Equal(t, "1.0.0", g.Metadata["version"])
	assert.Equal(t, "app.bicep", g.Metadata["sourceFile"])

	// Review-time properties are forwarded to the Node.
	frontend := g.Nodes[0]
	assert.Equal(t, "src/frontend.ts#L1", frontend.Properties["codeReference"])
	assert.Equal(t, int32(3), frontend.Properties["appDefinitionLine"])
	assert.Equal(t, "sha256:abc", frontend.Properties["diffHash"])
}

func TestMarshal_RoundTrip(t *testing.T) {
	t.Parallel()

	art := &StaticGraphArtifact{
		Version:     "1.0.0",
		GeneratedAt: "2026-05-27T00:00:00Z",
		SourceFile:  "app.bicep",
		Application: Application{
			Resources: []*Resource{
				{ID: "a", Type: "T", Name: "n", ProvisioningState: "Succeeded"},
			},
		},
	}

	data, err := Marshal(art)
	require.NoError(t, err)

	var back StaticGraphArtifact
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, art.Version, back.Version)
	assert.Equal(t, art.SourceFile, back.SourceFile)
	require.Len(t, back.Application.Resources, 1)
	assert.Equal(t, "a", back.Application.Resources[0].ID)
}

func TestMarshal_NilArtifact(t *testing.T) {
	t.Parallel()

	_, err := Marshal(nil)
	assert.Error(t, err)
}
