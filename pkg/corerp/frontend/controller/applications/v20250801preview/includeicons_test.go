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
	"bytes"
	"io"
	"net/http"
	"testing"

	corerpv20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_readGraphRequest enumerates the request-body shapes getGraph must
// tolerate. Both includeIcons and dependsOnEdges are additive on the wire, so
// absent / empty / non-JSON bodies must resolve to a zero-value struct rather
// than error. The returned pointer is always non-nil so callers can dereference
// fields without another nil check.
func Test_readGraphRequest(t *testing.T) {
	newRequest := func(body string, contentType string) *http.Request {
		var reader io.ReadCloser
		if body == "" {
			reader = http.NoBody
		} else {
			reader = io.NopCloser(bytes.NewBufferString(body))
		}
		req, err := http.NewRequest(http.MethodPost, "http://localhost/", reader)
		require.NoError(t, err)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if body != "" {
			req.ContentLength = int64(len(body))
		}
		return req
	}

	t.Run("nil body returns zero-value request", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, "http://localhost/", nil)
		require.NoError(t, err)
		got, err := readGraphRequest(req)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, to.Bool(got.IncludeIcons))
		assert.Nil(t, got.DependsOnEdges)
	})

	t.Run("empty body returns zero-value request", func(t *testing.T) {
		got, err := readGraphRequest(newRequest("", ""))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, to.Bool(got.IncludeIcons))
		assert.Nil(t, got.DependsOnEdges)
	})

	t.Run("body without JSON content type is ignored", func(t *testing.T) {
		got, err := readGraphRequest(newRequest(`{"includeIcons":true}`, "text/plain"))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, to.Bool(got.IncludeIcons))
	})

	t.Run("empty JSON object returns zero-value request", func(t *testing.T) {
		got, err := readGraphRequest(newRequest(`{}`, "application/json"))
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.False(t, to.Bool(got.IncludeIcons))
		assert.Nil(t, got.DependsOnEdges)
	})

	t.Run("includeIcons=true is honored", func(t *testing.T) {
		got, err := readGraphRequest(newRequest(`{"includeIcons":true}`, "application/json"))
		require.NoError(t, err)
		assert.True(t, to.Bool(got.IncludeIcons))
	})

	t.Run("includeIcons=false is honored", func(t *testing.T) {
		got, err := readGraphRequest(newRequest(`{"includeIcons":false}`, "application/json"))
		require.NoError(t, err)
		assert.False(t, to.Bool(got.IncludeIcons))
	})

	t.Run("dependsOnEdges is parsed", func(t *testing.T) {
		body := `{"dependsOnEdges":{"/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers/consumer":[{"id":"/planes/radius/local/resourceGroups/default/providers/Radius.Messaging/rabbitMQQueues/queue","direction":"Outbound","kind":"Dependency"}]}}`
		got, err := readGraphRequest(newRequest(body, "application/json"))
		require.NoError(t, err)
		require.NotNil(t, got.DependsOnEdges)
		require.Len(t, got.DependsOnEdges, 1)
		for _, entries := range got.DependsOnEdges {
			require.Len(t, entries, 1)
			assert.Equal(t, corerpv20250801preview.DirectionOutbound, *entries[0].Direction)
			assert.Equal(t, corerpv20250801preview.ConnectionKindDependency, *entries[0].Kind)
		}
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		_, err := readGraphRequest(newRequest(`{`, "application/json"))
		require.Error(t, err)
	})
}

// Test_buildIconsMap covers the response-side dedupe: every
// distinct hash referenced by the resources gets exactly one entry regardless
// of node count, with the verbatim SVG bytes attached.
func Test_buildIconsMap(t *testing.T) {
	svgA := "<svg>a</svg>"
	svgB := "<svg>b</svg>"
	icons := map[string]resourceTypeIcon{
		"Radius.Compute/containers":  {hash: "hash-a", bytes: svgA},
		"Radius.Data/mySqlDatabases": {hash: "hash-b", bytes: svgB},
	}

	t.Run("deduplicates icons keyed by hash", func(t *testing.T) {
		// Two container nodes share hash-a; a single mysql node has hash-b.
		resources := []*corerpv20250801preview.ApplicationGraphResource{
			{Type: to.Ptr("Radius.Compute/containers"), IconHash: to.Ptr("hash-a")},
			{Type: to.Ptr("Radius.Compute/containers"), IconHash: to.Ptr("hash-a")},
			{Type: to.Ptr("Radius.Data/mySqlDatabases"), IconHash: to.Ptr("hash-b")},
		}
		out := buildIconsMap(resources, icons)
		require.Len(t, out, 2, "one entry per distinct hash")
		require.Contains(t, out, "hash-a")
		require.Contains(t, out, "hash-b")
		assert.Equal(t, svgA, *out["hash-a"])
		assert.Equal(t, svgB, *out["hash-b"])
	})

	t.Run("skips nodes without an iconHash", func(t *testing.T) {
		resources := []*corerpv20250801preview.ApplicationGraphResource{
			{Type: to.Ptr("MyCompany.Test/widgets")}, // no hash
			{Type: to.Ptr("Radius.Compute/containers"), IconHash: to.Ptr("hash-a")},
		}
		out := buildIconsMap(resources, icons)
		require.Len(t, out, 1)
		require.Contains(t, out, "hash-a")
	})

	t.Run("returns nil when no icon bytes are available", func(t *testing.T) {
		// Nodes carry hashes but the icon lookup has no bytes (hash-only mode).
		hashOnly := map[string]resourceTypeIcon{
			"Radius.Compute/containers": {hash: "hash-a"},
		}
		resources := []*corerpv20250801preview.ApplicationGraphResource{
			{Type: to.Ptr("Radius.Compute/containers"), IconHash: to.Ptr("hash-a")},
		}
		out := buildIconsMap(resources, hashOnly)
		assert.Nil(t, out)
	})

	t.Run("returns nil for empty inputs", func(t *testing.T) {
		assert.Nil(t, buildIconsMap(nil, icons))
		assert.Nil(t, buildIconsMap([]*corerpv20250801preview.ApplicationGraphResource{}, icons))
		assert.Nil(t, buildIconsMap([]*corerpv20250801preview.ApplicationGraphResource{{Type: to.Ptr("x/y"), IconHash: to.Ptr("h")}}, nil))
	})
}
