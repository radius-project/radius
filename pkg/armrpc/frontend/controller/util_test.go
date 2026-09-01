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
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"uuid"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
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

func nextLinkContext(t *testing.T) context.Context {
	t.Helper()

	return v1.WithARMRequestContext(t.Context(), &v1.ARMRequestContext{
		APIVersion: "2025-08-01-preview",
		Top:        10,
	})
}

// nextLinkRequest models a request as the server receives it. inboundHost is the host the server
// saw, which behind the Kubernetes API server aggregator is the server's own in-cluster address
// rather than anything the client dialed.
func nextLinkRequest(t *testing.T, inboundHost string, path string) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "http://"+inboundHost+path+"?api-version=2025-08-01-preview&top=10", nil)
	require.NoError(t, err)
	return req
}

func TestGetNextLinkURL_IsRelativeSoTheClientSuppliesTheAddress(t *testing.T) {
	cases := []struct {
		name        string
		inboundHost string
		path        string
		expected    string
	}{
		{
			// A proxied resource provider: UCP has already trimmed its path base, so the path starts
			// at /planes/ and there is nothing left to strip.
			name:        "resource provider behind the UCP proxy",
			inboundHost: "dynamic-rp.radius-system:8082",
			path:        "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers",
			expected:    "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers?api-version=2025-08-01-preview&skipToken=page2&top=10",
		},
		{
			// UCP serving its own list: it mounts routes under its path base, which is already part
			// of every client's endpoint and so must not be repeated in the link.
			name:        "UCP serving its own list under a path base",
			inboundHost: "ucp.radius-system:9000",
			path:        "/apis/api.ucp.dev/v1alpha3/planes/radius/local/providers/System.Resources/resourceproviders",
			expected:    "/planes/radius/local/providers/System.Resources/resourceproviders?api-version=2025-08-01-preview&skipToken=page2&top=10",
		},
		{
			name:        "UCP serving an azure plane list under a path base",
			inboundHost: "ucp.radius-system:9000",
			path:        "/apis/api.ucp.dev/v1alpha3/subscriptions/s1/resourcegroups/rg/providers/Applications.Core/containers",
			expected:    "/subscriptions/s1/resourcegroups/rg/providers/Applications.Core/containers?api-version=2025-08-01-preview&skipToken=page2&top=10",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			req := nextLinkRequest(t, tt.inboundHost, tt.path)

			link := GetNextLinkURL(nextLinkContext(t), req, "page2")

			require.Equal(t, tt.expected, link)

			// The inbound host is not routable for the client, so it must never appear in the link.
			require.NotContains(t, link, tt.inboundHost)

			parsed, err := url.Parse(link)
			require.NoError(t, err)
			require.Empty(t, parsed.Scheme, "link must not be absolute")
			require.Empty(t, parsed.Host, "link must not carry an authority")

			// The link must be an absolute-path reference. azcore concatenates it onto the endpoint,
			// so a relative-path reference would silently produce a URL missing a separator.
			require.True(t, strings.HasPrefix(link, "/"), "link must start at the root")
		})
	}
}

func TestGetNextLinkURL_ResolvesAgainstTheEndpointTheClientDialed(t *testing.T) {
	// The endpoint a Kubernetes connection builds, whose path is exactly UCP's configured path base.
	const endpoint = "https://kubernetes.example.com/apis/api.ucp.dev/v1alpha3"

	cases := []struct {
		name string
		path string
	}{
		{
			name: "link produced by a proxied resource provider",
			path: "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers",
		},
		{
			name: "link produced by UCP itself",
			path: "/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			link := GetNextLinkURL(nextLinkContext(t), nextLinkRequest(t, "dynamic-rp.radius-system:8082", tt.path), "page2")

			// Resolve the link exactly as a generated client does, rather than modelling it. azcore
			// concatenates a scheme-less link onto the endpoint, which is not the same as RFC 3986
			// reference resolution: the latter would treat the link as replacing the endpoint's path
			// and drop the path base, producing a URL the API server does not route.
			req, err := runtime.NewRequestForNextLink(t.Context(), http.MethodGet, endpoint, link)
			require.NoError(t, err)

			// Whichever server produced the link, the client ends up at the same URL, and it is the
			// one UCP is served on through the aggregated APIService.
			require.Equal(t,
				"https://kubernetes.example.com/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers?api-version=2025-08-01-preview&skipToken=page2&top=10",
				req.Raw().URL.String())
		})
	}
}

func TestGetNextLinkURL_CarriesThePaginationState(t *testing.T) {
	req := nextLinkRequest(t, "dynamic-rp.radius-system:8082", "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers")

	link := GetNextLinkURL(nextLinkContext(t), req, "opaque token/with+reserved=chars")

	parsed, err := url.Parse(link)
	require.NoError(t, err)
	query := parsed.Query()

	require.Equal(t, "2025-08-01-preview", query.Get("api-version"))
	require.Equal(t, "10", query.Get("top"))
	// The token round trips through the encoding untouched.
	require.Equal(t, "opaque token/with+reserved=chars", query.Get("skipToken"))
}

func TestGetNextLinkURL_IsEmptyOnTheLastPage(t *testing.T) {
	req := nextLinkRequest(t, "dynamic-rp.radius-system:8082", "/planes/radius/local/resourcegroups/default/providers/Radius.Compute/containers")

	require.Empty(t, GetNextLinkURL(nextLinkContext(t), req, ""))
}
