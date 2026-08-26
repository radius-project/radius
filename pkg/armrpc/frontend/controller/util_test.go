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

package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/stretchr/testify/require"
)

func TestReadJSONBody(t *testing.T) {
	content, err := json.Marshal(map[string]string{
		"id":   "fakeID",
		"type": "fakeType",
	})
	require.NoError(t, err)

	contentTypeTests := []struct {
		contentType string
		body        []byte
		err         error
	}{
		{"application/json", content, nil},
		{"application/json; charset=utf8", content, nil},
		{"application/json;    charset=utf8", content, nil},
		{"Application/Json;    charset=utf8    ", content, nil},
		{"plain/text", content, ErrUnsupportedContentType},
	}

	for _, tc := range contentTypeTests {
		t.Run(tc.contentType, func(t *testing.T) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, "http://github.com", bytes.NewBuffer(tc.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", tc.contentType)
			// act
			parsed, err := ReadJSONBody(req)
			// assert
			if tc.err != nil {
				require.ErrorIs(t, tc.err, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, string(tc.body), string(parsed))
			}
		})
	}
}

var tag string = uuid.New().String()

func TestValidateEtag_IfMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		ifMatchEtag  string
		etagProvided string
		shouldFail   bool
	}{
		{
			"etag-provided-if-match-empty",
			"",
			"existingEtag",
			false,
		},
		{
			"etag-and-if-match-not-provided",
			"",
			"",
			false,
		},
		{
			"etag-if-match-provided-match",
			tag,
			tag,
			false,
		},
		{
			"etag-if-match-provided-no-match",
			tag,
			uuid.New().String(),
			true,
		},
		{
			"etag-not-provided-if-match-wildcard",
			"*",
			"",
			true,
		},
		{
			"etag-provided-if-match-wildcard",
			"*",
			tag,
			false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			armRequestContext := v1.ARMRequestContextFromContext(
				v1.WithARMRequestContext(
					t.Context(), &v1.ARMRequestContext{
						IfMatch: tt.ifMatchEtag,
					}))
			result := ValidateETag(*armRequestContext, tt.etagProvided)
			if !tt.shouldFail {
				require.Nil(t, result)
				require.NoError(t, result)
			} else {
				require.NotNil(t, result)
				require.Error(t, result)
			}
		})
	}
}

func TestValidateEtag_IfNoneMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		ifNoneMatchEtag string
		etagProvided    string
		shouldFail      bool
	}{
		{
			"etag-and-if-none-match-empty",
			"",
			"",
			false,
		},
		{
			"etag-provided-if-none-match-empty",
			"",
			tag,
			false,
		},
		{
			"etag-empty-if-none-match-wildcard",
			"*",
			"",
			false,
		},
		{
			"etag-provided-if-none-match-wildcard",
			"*",
			tag,
			true,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			armRequestContext := v1.ARMRequestContextFromContext(
				v1.WithARMRequestContext(
					t.Context(), &v1.ARMRequestContext{
						IfNoneMatch: tt.ifNoneMatchEtag,
					}))
			result := ValidateETag(*armRequestContext, tt.etagProvided)
			if !tt.shouldFail {
				require.Nil(t, result)
				require.NoError(t, result)
			} else {
				require.NotNil(t, result)
				require.Error(t, result)
			}
		})
	}
}

func TestGetURLFromReqWithQueryParameters(t *testing.T) {
	urlTests := []struct {
		name     string
		host     string
		scheme   string
		headers  map[string]string
		expected string
	}{
		{
			name:     "uses the request host when the request was not proxied",
			host:     "ucp.example.com",
			scheme:   "https",
			expected: "https://ucp.example.com/planes/radius/local?top=10",
		},
		{
			name:     "defaults the scheme when the request URL has none",
			host:     "ucp.example.com",
			expected: "http://ucp.example.com/planes/radius/local?top=10",
		},
		{
			name:   "prefers the forwarded host over the internal request host",
			host:   "dynamic-rp.radius-system:8082",
			scheme: "http",
			headers: map[string]string{
				"X-Forwarded-Host": "ucp.example.com",
			},
			expected: "http://ucp.example.com/planes/radius/local?top=10",
		},
		{
			name:   "prefers the forwarded scheme over the request scheme",
			host:   "ucp.example.com",
			scheme: "http",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
			},
			expected: "https://ucp.example.com/planes/radius/local?top=10",
		},
		{
			name:   "applies the forwarded host and scheme together",
			host:   "dynamic-rp.radius-system:8082",
			scheme: "http",
			headers: map[string]string{
				"X-Forwarded-Host":  "ucp.example.com",
				"X-Forwarded-Proto": "https",
			},
			expected: "https://ucp.example.com/planes/radius/local?top=10",
		},
		{
			name:   "uses the first entry of a forwarded header set by a proxy chain",
			host:   "dynamic-rp.radius-system:8082",
			scheme: "http",
			headers: map[string]string{
				"X-Forwarded-Host":  "ucp.example.com, inner.example.com",
				"X-Forwarded-Proto": "https, http",
			},
			expected: "https://ucp.example.com/planes/radius/local?top=10",
		},
		{
			name:   "falls back to the request values when the forwarded headers are empty",
			host:   "ucp.example.com",
			scheme: "https",
			headers: map[string]string{
				"X-Forwarded-Host":  "",
				"X-Forwarded-Proto": "",
			},
			expected: "https://ucp.example.com/planes/radius/local?top=10",
		},
	}

	for _, tc := range urlTests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "/planes/radius/local", nil)
			require.NoError(t, err)
			req.Host = tc.host
			req.URL.Scheme = tc.scheme
			for key, value := range tc.headers {
				req.Header.Set(key, value)
			}

			qps := url.Values{}
			qps.Add("top", "10")

			require.Equal(t, tc.expected, GetURLFromReqWithQueryParameters(req, qps).String())
		})
	}
}

func TestGetNextLinkURL(t *testing.T) {
	newRequest := func(t *testing.T) *http.Request {
		t.Helper()
		// A LIST that UCP proxied downstream: the provider sees its own
		// cluster-internal address as the host, and the caller-visible one only in
		// X-Forwarded-Host.
		req, err := http.NewRequest(http.MethodGet, "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers", nil)
		require.NoError(t, err)
		req.Host = "dynamic-rp.radius-system:8082"
		req.URL.Scheme = "http"
		return req
	}

	ctx := v1.WithARMRequestContext(t.Context(), &v1.ARMRequestContext{
		APIVersion: "2025-08-01-preview",
		Top:        10,
	})

	t.Run("returns an empty link when there are no further pages", func(t *testing.T) {
		require.Empty(t, GetNextLinkURL(ctx, newRequest(t), ""))
	})

	t.Run("returns a link the original caller can reach", func(t *testing.T) {
		req := newRequest(t)
		req.Header.Set("X-Forwarded-Host", "ucp.example.com")
		req.Header.Set("X-Forwarded-Proto", "https")

		nextLink, err := url.Parse(GetNextLinkURL(ctx, req, "skip-token"))
		require.NoError(t, err)

		require.Equal(t, "https", nextLink.Scheme)
		require.Equal(t, "ucp.example.com", nextLink.Host)
		require.Equal(t, "/planes/radius/local/resourceGroups/default/providers/Radius.Compute/containers", nextLink.Path)
		require.Equal(t, "2025-08-01-preview", nextLink.Query().Get("api-version"))
		require.Equal(t, "skip-token", nextLink.Query().Get("skipToken"))
		require.Equal(t, "10", nextLink.Query().Get("top"))
	})

	t.Run("does not leak the provider's cluster-internal address", func(t *testing.T) {
		req := newRequest(t)
		req.Header.Set("X-Forwarded-Host", "ucp.example.com")

		require.NotContains(t, GetNextLinkURL(ctx, req, "skip-token"), "dynamic-rp.radius-system")
	})

	t.Run("uses the request host when the request was not proxied", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "/planes/radius/local/providers/Radius.Compute/containers", nil)
		require.NoError(t, err)
		req.Host = "localhost:8080"
		req.URL.Scheme = "http"

		nextLink, err := url.Parse(GetNextLinkURL(ctx, req, "skip-token"))
		require.NoError(t, err)
		require.Equal(t, "localhost:8080", nextLink.Host)
	})
}
