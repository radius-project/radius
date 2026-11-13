// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomizeAuthorizationTypes(t *testing.T) {
	t.Parallel()
	raw := []byte(`[
		{"$type":"ObjectType","name":"role","properties":{"name":{"type":{"$ref":"#/4"},"flags":25}}},
		{"$type":"ObjectType","name":"assignment","properties":{"name":{"type":{"$ref":"#/4"},"flags":25}}},
		{"$type":"ResourceType","name":"Radius.Core/roleDefinitions@2025-08-01-preview","body":{"$ref":"#/0"},"readableScopes":0,"writableScopes":0},
		{"$type":"ResourceType","name":"Radius.Core/roleAssignments@2025-08-01-preview","body":{"$ref":"#/1"},"readableScopes":16,"writableScopes":16},
		{"$type":"StringType"}
	]`)
	first, err := customize(raw)
	require.NoError(t, err)
	second, err := customize(first)
	require.NoError(t, err)
	require.JSONEq(t, string(first), string(second))
	var nodes []node
	require.NoError(t, json.Unmarshal(first, &nodes))
	scopes, err := field[int](nodes[3], "writableScopes")
	require.NoError(t, err)
	require.Zero(t, scopes)
	properties, err := field[map[string]node](nodes[1], "properties")
	require.NoError(t, err)
	flags, err := field[int](properties["name"], "flags")
	require.NoError(t, err)
	require.Equal(t, 17, flags)
	require.Contains(t, properties, "scope")
	_, err = customize([]byte(`[]`))
	require.ErrorContains(t, err, "not both generated")
}
