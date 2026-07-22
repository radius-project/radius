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

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	productmanifest "github.com/radius-project/radius/deploy/manifest"
)

// Test_attachIconHashes_AttachesIconHashPerNode verifies the enricher
// attaches iconHash to every node. Types present in the fetched lookup
// contribute their per-type hash; types absent from the lookup (external
// cloud namespaces, unregistered types) fall back to the product default
// icon's hash.
func Test_attachIconHashes_AttachesIconHashPerNode(t *testing.T) {
	payload := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			{ID: to.Ptr("/planes/radius/local/.../containers/web"), Type: to.Ptr("Radius.Compute/containers"), Name: to.Ptr("web")},
			{ID: to.Ptr("/planes/radius/local/.../mySqlDatabases/db"), Type: to.Ptr("Radius.Data/mySqlDatabases"), Name: to.Ptr("db")},
			{ID: to.Ptr("/planes/radius/local/.../someType/none"), Type: to.Ptr("MyCompany.Test/widgets"), Name: to.Ptr("none")},
		},
	}
	icons := map[string]resourceTypeIcon{
		"Radius.Compute/containers":  {hash: "hash-containers"},
		"Radius.Data/mySqlDatabases": {hash: "hash-mysql"},
	}

	attachIconHashes(payload, icons)
	require.Len(t, payload.Resources, 3)

	require.NotNil(t, payload.Resources[0].IconHash)
	assert.Equal(t, "hash-containers", *payload.Resources[0].IconHash)

	require.NotNil(t, payload.Resources[1].IconHash)
	assert.Equal(t, "hash-mysql", *payload.Resources[1].IconHash)

	// Type not in the lookup falls back to the product default icon's hash
	// so every node in the response carries a resolvable icon.
	require.NotNil(t, payload.Resources[2].IconHash)
	assert.Equal(t, productmanifest.Default().Hash, *payload.Resources[2].IconHash)
}

// Test_attachIconHashes_NilIconsLookup asserts that when no per-type icons
// are available (nil map) every node still receives the product default
// icon's hash. The response's Icons map is populated only by buildIconsMap
// in the getGraph handler when includeIcons=true, so the enricher itself
// leaves it nil.
func Test_attachIconHashes_NilIconsLookup(t *testing.T) {
	payload := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			{ID: to.Ptr("id"), Type: to.Ptr("Radius.Compute/containers"), Name: to.Ptr("web")},
		},
	}

	attachIconHashes(payload, nil)
	require.Len(t, payload.Resources, 1)
	require.NotNil(t, payload.Resources[0].IconHash)
	assert.Equal(t, productmanifest.Default().Hash, *payload.Resources[0].IconHash)
	assert.Nil(t, payload.Icons)
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

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
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

	icons, err := fetchIcons(context.Background(), conn, &corerpv20250801preview.ApplicationGraphResponse{}, false)
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

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
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

// Test_fetchIcons_IntegrityCheck exercises the content-addressed
// integrity check on per-type icon bytes when includeBytes=true. Two
// types share the same graph:
//
//   - Radius.Compute/containers advertises hash H1 and returns bytes B1
//     where sha256(B1) == H1 — the entry survives with both hash and
//     bytes populated.
//   - Radius.Data/mySqlDatabases advertises hash H2 but returns bytes B2
//     where sha256(B2) != H2 — the entry is dropped entirely so
//     downstream code (attachIconHashes) falls back to the product
//     default for that type's nodes.
//
// The test also asserts fetchIcons does not fail the whole request on
// integrity mismatch — the graph must still render, just without an
// authoritative icon for the corrupted type.
func Test_fetchIcons_IntegrityCheck(t *testing.T) {
	const goodBytes = `<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`
	const goodHash = "cf34eb5c0999c5fce90abb88ee2a9afa9d71ac5ecd613169fb1d4d692441cb48"
	const corruptedBytes = "not the real svg"
	const advertisedHashOfCorrupted = "0000000000000000000000000000000000000000000000000000000000000000"

	mux := http.NewServeMux()
	mux.HandleFunc("/planes/radius/local/providers/Radius.Compute", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Radius.Compute",
			"resourceTypes": map[string]any{
				"containers": map[string]any{
					"iconHash": goodHash,
					"icon":     goodBytes,
				},
			},
		})
	})
	mux.HandleFunc("/planes/radius/local/providers/Radius.Data", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Radius.Data",
			"resourceTypes": map[string]any{
				"mySqlDatabases": map[string]any{
					"iconHash": advertisedHashOfCorrupted,
					"icon":     corruptedBytes,
				},
			},
		})
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	conn, err := sdk.NewDirectConnection(server.URL)
	require.NoError(t, err)

	graph := &corerpv20250801preview.ApplicationGraphResponse{
		Resources: []*corerpv20250801preview.ApplicationGraphResource{
			{Type: to.Ptr("Radius.Compute/containers")},
			{Type: to.Ptr("Radius.Data/mySqlDatabases")},
		},
	}

	icons, err := fetchIcons(context.Background(), conn, graph, true)
	require.NoError(t, err, "integrity failure must not fail the whole request")
	require.NotNil(t, icons)

	// Healthy type survives with hash and bytes.
	require.Contains(t, icons, "Radius.Compute/containers")
	assert.Equal(t, goodHash, icons["Radius.Compute/containers"].hash)
	assert.Equal(t, goodBytes, icons["Radius.Compute/containers"].bytes)

	// Corrupted type is absent from the map. Downstream attachIconHashes
	// will fall through to productmanifest.DefaultHash() for nodes of
	// this type, and buildIconsMap will emit the default bytes under the
	// default hash — exactly the fallback path that already handles
	// "type registered without an icon."
	assert.NotContains(t, icons, "Radius.Data/mySqlDatabases",
		"integrity check must drop the corrupted entry so the default fallback fires")
}
