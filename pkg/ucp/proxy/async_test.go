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

package proxy

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_ProcessAsyncOperationHeaders(t *testing.T) {
	originalAsyncOperationHeader := "http://localhost:7443/async-operation-url?query=yeah"
	originalLocationHeader := "http://localhost:7443/location-url?query=yeah"
	createTestRequest := func(t *testing.T) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://localhost:9443/downstream-url", nil)
		req.Header.Set(v1.RefererHeader, "http://localhost:9443/planes/test/local/downstream-url")

		ctx := testcontext.New(t)
		req = req.WithContext(ctx)
		return req
	}
	createTestResponse := func(t *testing.T, req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Request:    req,
			Header: http.Header{
				azureAsyncOperationHeader: []string{originalAsyncOperationHeader},
				locationHeader:            []string{originalLocationHeader},
			},
		}
	}

	t.Run("positive", func(t *testing.T) {
		req := createTestRequest(t)
		resp := createTestResponse(t, req)
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, "http://localhost:9443/planes/test/local/async-operation-url?query=yeah", resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, "http://localhost:9443/planes/test/local/location-url?query=yeah", resp.Header.Get(locationHeader))
	})

	t.Run("positive path base", func(t *testing.T) {
		req := createTestRequest(t)
		req.Header.Set(v1.RefererHeader, "http://localhost:9443/path/base/planes/test/local/downstream-url")
		resp := createTestResponse(t, req)
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, "http://localhost:9443/path/base/planes/test/local/async-operation-url?query=yeah", resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, "http://localhost:9443/path/base/planes/test/local/location-url?query=yeah", resp.Header.Get(locationHeader))
	})

	t.Run("wrong-status-code", func(t *testing.T) {
		req := createTestRequest(t)
		resp := createTestResponse(t, req)
		resp.StatusCode = http.StatusNoContent
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, originalAsyncOperationHeader, resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, originalLocationHeader, resp.Header.Get(locationHeader))
	})

	t.Run("no referrer", func(t *testing.T) {
		req := createTestRequest(t)
		req.Header.Del(v1.RefererHeader)
		resp := createTestResponse(t, req)
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, originalAsyncOperationHeader, resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, originalLocationHeader, resp.Header.Get(locationHeader))
	})

	t.Run("invalid referrer", func(t *testing.T) {
		req := createTestRequest(t)
		req.Header.Set(v1.RefererHeader, "\ninvalid-referrer")
		resp := createTestResponse(t, req)
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, originalAsyncOperationHeader, resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, originalLocationHeader, resp.Header.Get(locationHeader))
	})

	t.Run("non-UCP referrer", func(t *testing.T) {
		req := createTestRequest(t)
		req.Header.Set(v1.RefererHeader, "http://example.com")
		resp := createTestResponse(t, req)
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, originalAsyncOperationHeader, resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, originalLocationHeader, resp.Header.Get(locationHeader))
	})

	t.Run("invalid response header", func(t *testing.T) {
		req := createTestRequest(t)
		resp := createTestResponse(t, req)
		resp.Header.Set(azureAsyncOperationHeader, "\ninvalid-header")
		resp.Header.Set(locationHeader, "\ninvalid-header")
		err := ProcessAsyncOperationHeaders(resp)
		require.NoError(t, err)

		require.Equal(t, "\ninvalid-header", resp.Header.Get(azureAsyncOperationHeader))
		require.Equal(t, "\ninvalid-header", resp.Header.Get(locationHeader))
	})
}

func Test_rewriteHeader(t *testing.T) {
	t.Run("positive", func(t *testing.T) {
		type positiveTest []struct {
			header   string
			referrer string
			pathBase string
			expected string
		}
		positiveTestData := positiveTest{
			// Downsteam header is the original request path without the planes prefix.
			{
				header:   "http://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				referrer: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				expected: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			},

			// Downsteam header is the original request path with the planes prefix.
			{
				header:   "http://localhost:7443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				referrer: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				expected: "http://localhost:9443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			},

			// (path base) Downsteam header is the original request path without the planes prefix.
			{
				header:   "http://localhost:7443/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				referrer: "http://localhost:9443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				pathBase: "/path/base",
				expected: "http://localhost:9443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			},

			// (path base) Downsteam header is the original request path with the planes prefix.
			{
				header:   "http://localhost:7443/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				referrer: "http://localhost:9443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				pathBase: "/path/base",
				expected: "http://localhost:9443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			},

			// (path base) Downsteam header is the original request path with the path base planes prefix.
			{
				header:   "http://localhost:7443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				referrer: "http://localhost:9443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
				pathBase: "/path/base",
				expected: "http://localhost:9443/path/base/planes/test/local/subscriptions/sid/resourceGroups/rg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application/testApp/Container/test",
			},

			// Downsteam header has a query string and fragment
			{
				header:   "http://localhost:7443/abcd?efgh=value#ijkl",
				referrer: "http://localhost:9443/planes/test/local/abcd",
				expected: "http://localhost:9443/planes/test/local/abcd?efgh=value",
			},

			// Downsteam header has a different scheme
			{
				header:   "http://localhost:7443/abcd",
				referrer: "https://localhost:9443/planes/test/local/abcd",
				expected: "https://localhost:9443/planes/test/local/abcd",
			},

			// Downsteam header has an empty path
			{
				header:   "http://localhost:7443",
				referrer: "https://localhost:9443/planes/test/local/abcd",
				expected: "https://localhost:9443/planes/test/local",
			},
		}
		for _, datum := range positiveTestData {
			referrerURL, err := url.Parse(datum.referrer)
			require.NoError(t, err)

			planeType, planeName, _, err := resources.ExtractPlanesPrefixFromURLPath(referrerURL.Path[len(datum.pathBase):])
			require.NoError(t, err)

			result, err := rewriteHeader(datum.header, referrerURL, datum.pathBase, fmt.Sprintf("/%s/%s/%s", resources.PlanesSegment, planeType, planeName))
			require.NoError(t, err, "%q should have not have failed", datum)
			require.Equal(t, datum.expected, result)
		}
	})

	t.Run("negative", func(t *testing.T) {
		type negativeTest []struct {
			header      string
			referrer    string
			expectedErr string
		}
		negativeTestData := negativeTest{
			// It's actually HARD to construct an invalid URL in Go.
			//
			// Most of the error handling for the header rewrite logic is done before calling rewriteHeader.
			{
				header:      "\n?not-a-url-http://////",
				referrer:    "https://localhost:9443/planes/test/local/abcd",
				expectedErr: "header value is not a valid URL: parse \"\\n?not-a-url-http://////\": net/url: invalid control character in URL",
			},
		}
		for _, datum := range negativeTestData {
			referrerURL, err := url.Parse(datum.referrer)
			require.NoError(t, err)

			planeType, planeName, _, err := resources.ExtractPlanesPrefixFromURLPath(referrerURL.Path)
			require.NoError(t, err)

			result, err := rewriteHeader(datum.header, referrerURL, "", fmt.Sprintf("/%s/%s/%s", resources.PlanesSegment, planeType, planeName))
			require.Errorf(t, err, "%q should have failed", datum)
			require.Equal(t, "", result)
			require.Equal(t, datum.expectedErr, err.Error())
		}
	})
}
