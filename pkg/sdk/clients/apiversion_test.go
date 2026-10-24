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

package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/radius-project/radius/pkg/sdk"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAPIVersion(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		versions   []string
		defaultAPI *string
		typeName   string
		nilType    bool
		status     int
		expected   string
		errorText  string
	}{
		{name: "modern", versions: []string{"2025-08-01-preview"}, expected: "2025-08-01-preview"},
		{name: "legacy", versions: []string{"2023-10-01-preview"}, expected: "2023-10-01-preview"},
		{name: "arbitrary label", versions: []string{"release-blue"}, expected: "release-blue"},
		{name: "default overrides ordering", versions: []string{"alpha", "zeta"}, defaultAPI: new("zeta"), expected: "zeta"},
		{name: "stable not semantic ordering", versions: []string{"v2", "v10", "v3"}, expected: "v10"},
		{name: "empty default", versions: []string{"zeta", "alpha"}, defaultAPI: new(""), expected: "alpha"},
		{name: "ignore empty key", versions: []string{"", "v1"}, expected: "v1"},
		{name: "invalid default", versions: []string{"v1"}, defaultAPI: new("v2"), errorText: `default API version "v2"`},
		{name: "opaque default casing", versions: []string{"v1"}, defaultAPI: new("V1"), errorText: `default API version "V1"`},
		{name: "missing provider", status: http.StatusNotFound, errorText: `could not get resource provider "Contoso.Test" in plane "remote"`},
		{name: "missing type", typeName: "other", errorText: `resource type "widgets" not found`},
		{name: "null type", nilType: true, errorText: `resource type "widgets" not found`},
		{name: "no versions", errorText: "no supported API versions"},
		{name: "only empty key", versions: []string{""}, errorText: "no supported API versions"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			typeName := tt.typeName
			if typeName == "" {
				typeName = "WIDGETS"
			}
			rt := &ucpv20231001preview.ResourceProviderSummaryResourceType{
				APIVersions:       map[string]*ucpv20231001preview.ResourceTypeSummaryResultAPIVersion{},
				DefaultAPIVersion: tt.defaultAPI,
			}
			for _, version := range tt.versions {
				rt.APIVersions[version] = &ucpv20231001preview.ResourceTypeSummaryResultAPIVersion{}
			}
			if tt.nilType {
				rt = nil
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/planes/radius/remote/providers/Contoso.Test", r.URL.Path)
				assert.Equal(t, "2023-10-01-preview", r.URL.Query().Get("api-version"), "provider metadata uses the UCP API version")
				w.Header().Set("Content-Type", "application/json")
				if tt.status != 0 {
					w.WriteHeader(tt.status)
					return
				}
				assert.NoError(t, json.NewEncoder(w).Encode(ucpv20231001preview.ResourceProviderSummary{
					ResourceTypes: map[string]*ucpv20231001preview.ResourceProviderSummaryResourceType{typeName: rt},
				}))
			}))
			t.Cleanup(server.Close)
			connection, err := sdk.NewDirectConnection(server.URL)
			require.NoError(t, err)
			id, err := resources.ParseResource("/planes/RADIUS/remote/resourceGroups/test/providers/Contoso.Test/widgets/one")
			require.NoError(t, err)
			version, err := ResolveAPIVersion(t.Context(), connection, id)
			if tt.errorText != "" {
				require.ErrorContains(t, err, tt.errorText)
				require.Empty(t, version)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, version)
			}
		})
	}
}

func TestResolveAPIVersion_NonRadius(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm",
		"/planes/azure/cloud/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm",
		"/planes/aws/aws/accounts/123/regions/us-east-1/providers/AWS.S3/Bucket/one",
		"/planes/kubernetes/local/namespaces/default/providers/core/Secret/one",
	} {
		t.Run(raw, func(t *testing.T) {
			id, err := resources.ParseResource(raw)
			require.NoError(t, err)
			_, err = ResolveAPIVersion(t.Context(), nil, id)
			require.ErrorContains(t, err, "outside a Radius plane")
		})
	}
}

func TestResolveAPIVersion_TransportError(t *testing.T) {
	t.Parallel()
	connection, err := sdk.NewDirectConnection("http://localhost")
	require.NoError(t, err)
	connection = apiVersionFailingConnection{Connection: connection}
	id, err := resources.ParseResource("/planes/radius/local/resourceGroups/test/providers/Contoso.Test/widgets/one")
	require.NoError(t, err)
	_, err = ResolveAPIVersion(t.Context(), connection, id)
	require.ErrorIs(t, err, context.Canceled)
}

type apiVersionFailingTransport struct{}

func (apiVersionFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.Canceled
}

type apiVersionFailingConnection struct {
	sdk.Connection
}

func (apiVersionFailingConnection) Client() *http.Client {
	return &http.Client{Transport: apiVersionFailingTransport{}}
}
