/*
Copyright 2026 The Radius Authors.

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

package radius

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/stretchr/testify/require"
)

func TestDelegatedInventoryPreservesLogicalMembersOnly(t *testing.T) {
	t.Parallel()
	const scope = "/planes/radius/local/resourceGroups/rg/providers/"
	target := scope + "Radius.Compute/containers/target"
	delegate := scope + "Radius.Compute/routes/route"
	store := inmemory.NewClient()
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: target, Data: map[string]any{
		"properties": map[string]any{"status": map[string]any{
			"delegatedResources": []any{map[string]any{"id": delegate, "type": "Radius.Compute/routes", "apiVersion": "2025-08-01-preview"}},
			"outputResources":    []any{map[string]any{"id": "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.App/containerApps/native"}},
		}},
	}}))
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(
		`{"properties":{"outputResources":[{"id":"` + target + `"}],"outputs":{"number":{"value":9007199254740993}}}}`,
	))}
	require.NoError(t, expandDelegatedDeploymentOutputs(t.Context(), store, response))
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Contains(t, string(data), delegate)
	require.Contains(t, string(data), "9007199254740993", "inventory projection must not change unrelated output precision")
	require.NotContains(t, string(data), "containerApps/native")
	require.Equal(t, int64(len(data)), response.ContentLength)

	expanded, err := addDelegatedOutputResources(t.Context(), store, []any{
		map[string]any{"id": target}, map[string]any{"id": delegate},
	})
	require.NoError(t, err)
	require.Len(t, expanded, 2)
}

func TestDelegatedInventoryRejectsCrossScopeMember(t *testing.T) {
	t.Parallel()
	target := "/planes/radius/local/resourceGroups/rg/providers/Radius.Compute/containers/target"
	store := inmemory.NewClient()
	require.NoError(t, store.Save(t.Context(), &database.Object{ID: target, Data: map[string]any{
		"properties": map[string]any{"status": map[string]any{"delegatedResources": []any{map[string]any{
			"id":   "/planes/radius/local/resourceGroups/other/providers/Radius.Compute/routes/route",
			"type": "Radius.Compute/routes", "apiVersion": "2025-08-01-preview",
		}}}},
	}}))
	_, err := addDelegatedOutputResources(t.Context(), store, []any{map[string]any{"id": target}})
	require.ErrorContains(t, err, "invalid logical delegated output inventory")
}
