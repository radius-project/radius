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

package clients

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type stubConnection struct {
	endpoint string
	client   *http.Client
}

func (s stubConnection) Client() *http.Client {
	return s.client
}

func (s stubConnection) Endpoint() string {
	return s.endpoint
}

func TestResolveAPIVersion(t *testing.T) {
	t.Parallel()

	type summaryPayload struct {
		Name          string `json:"name"`
		ResourceTypes map[string]*struct {
			DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
			APIVersions       map[string]any         `json:"apiVersions,omitempty"`
			Capabilities      []string               `json:"capabilities,omitempty"`
			Description       *string                `json:"description,omitempty"`
			Extra             map[string]interface{} `json:"-"`
		} `json:"resourceTypes,omitempty"`
	}

	tests := []struct {
		name        string
		id          string
		plane       string
		provider    string
		statusCode  int
		summary     any
		wantVersion string
		wantErr     string
	}{
		{
			name:       "modern only",
			id:         "/planes/radius/local/resourceGroups/default/providers/Radius.Core/environments/dev",
			plane:      "local",
			provider:   "Radius.Core",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "Radius.Core",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"applications": {
						APIVersions: map[string]any{
							"2025-08-01-preview": map[string]any{},
						},
					},
					"environments": {
						APIVersions: map[string]any{
							"2025-08-01-preview": map[string]any{},
						},
					},
				},
			},
			wantVersion: "2025-08-01-preview",
		},
		{
			name:       "legacy only",
			id:         "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/app",
			plane:      "local",
			provider:   "Applications.Core",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "Applications.Core",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"containers": {
						APIVersions: map[string]any{
							"2023-10-01-preview": map[string]any{},
						},
					},
				},
			},
			wantVersion: "2023-10-01-preview",
		},
		{
			name:       "arbitrary UDT version",
			id:         "/planes/radius/custom/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "custom",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"Widgets": {
						APIVersions: map[string]any{
							"v1alpha1": map[string]any{},
						},
					},
				},
			},
			wantVersion: "v1alpha1",
		},
		{
			name:       "default API version takes precedence",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"widgets": {
						DefaultAPIVersion: new("2025-01-01"),
						APIVersions: map[string]any{
							"2024-01-01": map[string]any{},
							"2025-01-01": map[string]any{},
						},
					},
				},
			},
			wantVersion: "2025-01-01",
		},
		{
			name:       "lexicographical fallback when default is absent",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"widgets": {
						APIVersions: map[string]any{
							"2025-08-01-preview": map[string]any{},
							"2023-10-01-preview": map[string]any{},
						},
					},
				},
			},
			wantVersion: "2023-10-01-preview",
		},
		{
			name:       "empty default falls back to advertised version",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"widgets": {
						DefaultAPIVersion: new(""),
						APIVersions: map[string]any{
							"2025-08-01-preview": map[string]any{},
						},
					},
				},
			},
			wantVersion: "2025-08-01-preview",
		},
		{
			name:       "default API version not in advertised versions",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"widgets": {
						DefaultAPIVersion: new("2026-01-01"),
						APIVersions: map[string]any{
							"2025-01-01": map[string]any{},
						},
					},
				},
			},
			wantErr: `default API version "2026-01-01" for type "MyCompany.Custom/widgets" is not advertised`,
		},
		{
			name:       "resource type has no supported versions",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"widgets": {
						APIVersions: map[string]any{},
					},
				},
			},
			wantErr: `resource type "MyCompany.Custom/widgets" has no supported API versions`,
		},
		{
			name:       "resource type missing from provider summary",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusOK,
			summary: summaryPayload{
				Name: "MyCompany.Custom",
				ResourceTypes: map[string]*struct {
					DefaultAPIVersion *string                `json:"defaultApiVersion,omitempty"`
					APIVersions       map[string]any         `json:"apiVersions,omitempty"`
					Capabilities      []string               `json:"capabilities,omitempty"`
					Description       *string                `json:"description,omitempty"`
					Extra             map[string]interface{} `json:"-"`
				}{
					"gadgets": {
						APIVersions: map[string]any{
							"2025-08-01-preview": map[string]any{},
						},
					},
				},
			},
			wantErr: `resource type "MyCompany.Custom/widgets" was not found in provider "MyCompany.Custom"`,
		},
		{
			name:       "provider summary returns 404",
			id:         "/planes/radius/local/resourceGroups/default/providers/MyCompany.Custom/widgets/w1",
			plane:      "local",
			provider:   "MyCompany.Custom",
			statusCode: http.StatusNotFound,
			summary: map[string]any{
				"error": map[string]any{
					"code":    "NotFound",
					"message": "provider not found",
				},
			},
			wantErr: "404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath, gotAPIVersion string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAPIVersion = r.URL.Query().Get("api-version")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(tt.summary)
			}))
			defer srv.Close()

			conn, err := sdk.NewDirectConnection(srv.URL)
			require.NoError(t, err)

			id, err := resources.ParseResource(tt.id)
			require.NoError(t, err)

			version, err := ResolveAPIVersion(context.Background(), conn, id)
			require.Equal(t, "/planes/radius/"+tt.plane+"/providers/"+tt.provider, gotPath)
			require.Equal(t, "2023-10-01-preview", gotAPIVersion)

			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantVersion, version)
		})
	}
}

func TestResolveAPIVersion_RejectsNonRadiusPlanes(t *testing.T) {
	t.Parallel()

	conn, err := sdk.NewDirectConnection("http://localhost:9000")
	require.NoError(t, err)

	for _, rawID := range []string{
		"/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/sa",
		"/planes/aws/aws/accounts/123456789012/regions/us-west-2/providers/AWS.S3/Bucket/my-bucket",
		"/planes/kubernetes/local/namespaces/default/providers/core/Service/my-svc",
	} {
		id, err := resources.ParseResource(rawID)
		require.NoError(t, err)

		_, err = ResolveAPIVersion(context.Background(), conn, id)
		require.ErrorContains(t, err, "is not in a Radius plane")
	}
}

func TestResolveAPIVersion_TransportError(t *testing.T) {
	t.Parallel()

	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/default/providers/Radius.Core/environments/dev")
	require.NoError(t, err)

	conn := stubConnection{
		endpoint: "http://example.invalid",
		client: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("connection refused")
			}),
		},
	}

	_, err = ResolveAPIVersion(context.Background(), conn, id)
	require.ErrorContains(t, err, "connection refused")
}

func TestResolveAPIVersion_ClientInitError(t *testing.T) {
	t.Parallel()

	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/default/providers/Radius.Core/environments/dev")
	require.NoError(t, err)

	_, err = ResolveAPIVersion(context.Background(), stubConnection{}, id)
	require.Error(t, err)
}
