// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package graph

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	in := &Graph{
		ID:   "app1",
		Name: "myapp",
		Nodes: []Node{
			{ID: "a", Type: "T", Name: "A"},
			{ID: "b", Type: "T", Name: "B"},
		},
		Edges: []Edge{
			{Source: "a", Target: "b", Kind: "Outbound"},
		},
		Metadata: map[string]string{"version": "1.0.0"},
	}

	data, err := in.ToJSON()
	require.NoError(t, err)

	out, err := FromJSON(data)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestFromJSON_EmptyInput(t *testing.T) {
	t.Parallel()

	_, err := FromJSON(nil)
	assert.Error(t, err)
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	t.Parallel()

	_, err := FromJSON([]byte("{not-json"))
	assert.Error(t, err)
}

func TestToJSON_ProducesValidJSON(t *testing.T) {
	t.Parallel()

	g := &Graph{ID: "x"}
	data, err := g.ToJSON()
	require.NoError(t, err)

	var any map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &any))
	assert.Equal(t, "x", any["id"])
}
