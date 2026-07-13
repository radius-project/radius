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

package v20250801preview

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_splitResourceType covers the fully-qualified resource-type parser used
// to bucket graph nodes by their provider namespace before we call
// GetProviderSummary.
func Test_splitResourceType(t *testing.T) {
	cases := []struct {
		in          string
		namespace   string
		typeName    string
		ok          bool
		explanation string
	}{
		{"Radius.Compute/containers", "Radius.Compute", "containers", true, "well-formed"},
		{"Radius.Core/applications", "Radius.Core", "applications", true, "well-formed built-in"},
		{"", "", "", false, "empty input"},
		{"NoSlash", "", "", false, "missing separator"},
		{"/containers", "", "", false, "empty namespace"},
		{"Radius.Compute/", "", "", false, "empty type name"},
	}
	for _, c := range cases {
		t.Run(c.explanation, func(t *testing.T) {
			ns, tn, ok := splitResourceType(c.in)
			assert.Equal(t, c.ok, ok)
			assert.Equal(t, c.namespace, ns)
			assert.Equal(t, c.typeName, tn)
		})
	}
}

// Test_convertGraphResponseWithIcons_AttachesIconHashPerNode verifies that the
// v20231001preview → v20250801preview conversion attaches iconHash to the
// matching node and leaves it nil for types that have no icon in the lookup.
func Test_convertGraphResponseWithIcons_AttachesIconHashPerNode(t *testing.T) {
	payload := &corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{ID: to.Ptr("/planes/radius/local/.../containers/web"), Type: to.Ptr("Radius.Compute/containers"), Name: to.Ptr("web")},
			{ID: to.Ptr("/planes/radius/local/.../mySqlDatabases/db"), Type: to.Ptr("Radius.Data/mySqlDatabases"), Name: to.Ptr("db")},
			{ID: to.Ptr("/planes/radius/local/.../someType/none"), Type: to.Ptr("MyCompany.Test/widgets"), Name: to.Ptr("none")},
		},
	}
	icons := map[string]resourceTypeIcon{
		"Radius.Compute/containers":  {hash: "hash-containers"},
		"Radius.Data/mySqlDatabases": {hash: "hash-mysql"},
	}

	out := convertGraphResponseWithIcons(payload, icons)
	require.Len(t, out.Resources, 3)

	require.NotNil(t, out.Resources[0].IconHash)
	assert.Equal(t, "hash-containers", *out.Resources[0].IconHash)

	require.NotNil(t, out.Resources[1].IconHash)
	assert.Equal(t, "hash-mysql", *out.Resources[1].IconHash)

	// Type not in the lookup gets a nil hash — never a placeholder or empty string.
	assert.Nil(t, out.Resources[2].IconHash)
}

// Test_convertGraphResponseWithIcons_NilIconsLookup asserts the converter is a
// pure passthrough when no icon lookup is available.
func Test_convertGraphResponseWithIcons_NilIconsLookup(t *testing.T) {
	payload := &corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{ID: to.Ptr("id"), Type: to.Ptr("Radius.Compute/containers"), Name: to.Ptr("web")},
		},
	}

	out := convertGraphResponseWithIcons(payload, nil)
	require.Len(t, out.Resources, 1)
	assert.Nil(t, out.Resources[0].IconHash)
	assert.Nil(t, out.Icons)
}

// Test_fetchIcons_HashOnly stands up a fake UCP provider-summary server, asks
// fetchIcons for hash-only mode, and asserts we get one entry per registered
// type, keyed by "<namespace>/<typeName>", with bytes left empty.
func Test_fetchIcons_HashOnly(t *testing.T) {
	// Fake UCP /planes/radius/local/providers/<ns> summary endpoint.
	// Only Radius.Compute has a registered icon here; Radius.Data has none.
	mux := http.NewServeMux()
	mux.HandleFunc("/planes/radius/local/providers/Radius.Compute", func(w http.ResponseWriter, r *http.Request) {
		// Regardless of includeIcons value, hash-only mode should not request bytes.
		assert.NotEqual(t, "true", r.URL.Query().Get("includeIcons"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Radius.Compute",
			"resourceTypes": map[string]any{
				"containers": map[string]any{
					"iconHash": "hash-containers",
				},
			},
		})
	})
	mux.HandleFunc("/planes/radius/local/providers/Radius.Data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name":          "Radius.Data",
			"resourceTypes": map[string]any{"mySqlDatabases": map[string]any{}},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	graph := &corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{Type: to.Ptr("Radius.Compute/containers")},
			{Type: to.Ptr("Radius.Data/mySqlDatabases")},
		},
	}

	icons, err := fetchIcons(context.Background(), conn, graph, false)
	require.NoError(t, err)
	require.NotNil(t, icons)

	// Only the type with a non-nil iconHash comes back.
	require.Contains(t, icons, "Radius.Compute/containers")
	assert.Equal(t, "hash-containers", icons["Radius.Compute/containers"].hash)
	assert.Empty(t, icons["Radius.Compute/containers"].bytes)

	// Types without an iconHash are not in the map.
	assert.NotContains(t, icons, "Radius.Data/mySqlDatabases")
}

// Test_fetchIcons_EmptyGraph exercises the fast-path exit for a graph with no
// resources — we must not make any HTTP calls.
func Test_fetchIcons_EmptyGraph(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("fetchIcons must not call UCP for an empty graph; got %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	conn, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	icons, err := fetchIcons(context.Background(), conn, &corerpv20231001preview.ApplicationGraphResponse{}, false)
	require.NoError(t, err)
	assert.Nil(t, icons)
}

// Test_fetchIcons_ExternalProviderNotFound covers the case where the graph
// contains connected external nodes (e.g. Microsoft.Storage/storageAccounts)
// whose provider is not registered in the local Radius resource-type registry.
// GetProviderSummary returns 404 for that namespace; the graph request must
// still succeed, and the icons map should carry entries only for the
// namespaces that were resolvable.
func Test_fetchIcons_ExternalProviderNotFound(t *testing.T) {
	mux := http.NewServeMux()
	// Radius.Compute is registered locally and has an icon.
	mux.HandleFunc("/planes/radius/local/providers/Radius.Compute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Radius.Compute",
			"resourceTypes": map[string]any{
				"containers": map[string]any{
					"iconHash": "hash-containers",
				},
			},
		})
	})
	// Microsoft.Storage has no handler — the mux answers 404, mirroring what
	// UCP returns for an unregistered provider namespace.
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	graph := &corerpv20231001preview.ApplicationGraphResponse{
		Resources: []*corerpv20231001preview.ApplicationGraphResource{
			{Type: to.Ptr("Radius.Compute/containers")},
			{Type: to.Ptr("Microsoft.Storage/storageAccounts")},
		},
	}

	// includeBytes=false because the reviewer's report was specifically that
	// the graph errored even when the caller had not opted into inline bytes.
	icons, err := fetchIcons(context.Background(), conn, graph, false)
	require.NoError(t, err)
	require.NotNil(t, icons)

	// The resolvable namespace contributes its icon; the missing one is
	// silently skipped so the corresponding node's IconHash ends up nil.
	require.Contains(t, icons, "Radius.Compute/containers")
	assert.Equal(t, "hash-containers", icons["Radius.Compute/containers"].hash)
	assert.NotContains(t, icons, "Microsoft.Storage/storageAccounts")
}
