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

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		armid    string
		expected string
	}{
		{
			"/Planes",
			"/planes",
		},
		{
			"/Planes/radius/Local/resourceGroups",
			"/planes/radius/Local/resourcegroups",
		},
		{
			// Tests when /planes is not in the beginning
			"/apis/api.ucp.dev/v1alpha3/Planes/Radius/Local",
			"/apis/api.ucp.dev/v1alpha3/planes/Radius/Local",
		},
		{
			"/Planes/radius/local/ResourceGroups/abc/providers/Applications.Core/environments/Default",
			"/planes/radius/local/resourcegroups/abc/providers/Applications.Core/environments/Default",
		},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		r := chi.NewRouter()
		r.MethodFunc(
			http.MethodPost,
			"/planes/{planeType}/{planeName}/resourcegroups/{resourceGroupName}/providers/Applications.Core/{resourceType}/{resourceName}",
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		r.MethodFunc(
			http.MethodPost,
			"/planes",
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		r.MethodFunc(
			http.MethodPost,
			"/planes/{planeType}/{planeName}/resourcegroups",
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		r.MethodFunc(
			http.MethodPost,
			"/apis/api.ucp.dev/v1alpha3/planes/{planeType}/{planeName}",
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})

		handler := NormalizePath(r)

		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, tt.armid, nil)
		require.NoError(t, err)
		handler.ServeHTTP(w, req)

		parsed := w.Body.String()
		require.Equal(t, tt.expected, parsed)
	}
}

func Test_NormalizePath_SegmentRewrite(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "ascii",
			path:     "/Planes/radius/Kteam/ResourceGroups/team",
			expected: "/planes/radius/Kteam/resourcegroups/team",
		},
		{
			name:     "mixed case segments",
			path:     "/apis/api.ucp.dev/v1alpha3/pLaNeS/radius/local/rEsOuRcEgRoUpS/Abc",
			expected: "/apis/api.ucp.dev/v1alpha3/planes/radius/local/resourcegroups/Abc",
		},
		{
			name:     "lowercasing shrinks preceding character",
			path:     "/planes/radius/\u212Ateam/resourceGroups/team",
			expected: "/planes/radius/\u212Ateam/resourcegroups/team",
		},
		{
			name:     "lowercasing grows preceding character",
			path:     "/planes/radius/\u023Ateam/resourceGroups/team",
			expected: "/planes/radius/\u023Ateam/resourcegroups/team",
		},
		{
			name:     "non-ascii before planes",
			path:     "/\u212A\u023A/Planes/radius/local",
			expected: "/\u212A\u023A/planes/radius/local",
		},
		{
			name:     "only first occurrence is rewritten",
			path:     "/Planes/radius/local/ResourceGroups/Planes/providers/Foo/ResourceGroups/x",
			expected: "/planes/radius/local/resourcegroups/Planes/providers/Foo/ResourceGroups/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got string
			handler := NormalizePath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Path
			}))

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			req.URL.Path = tt.path
			handler.ServeHTTP(httptest.NewRecorder(), req)

			require.Equal(t, tt.expected, got)
		})
	}
}

func Test_NormalizePath_EscapedRequestTarget(t *testing.T) {
	tests := []struct {
		name            string
		target          string
		expectedPath    string
		expectedRawPath string
		expectedName    string
	}{
		{
			name:            "default escaping",
			target:          "/Planes/radius/%C8%BAteam/resourceGroups/team",
			expectedPath:    "/planes/radius/\u023Ateam/resourcegroups/team",
			expectedRawPath: "",
			expectedName:    "\u023Ateam",
		},
		{
			name:            "non-default escaping",
			target:          "/Planes/radius/%c8%bateam/resourceGroups/team",
			expectedPath:    "/planes/radius/\u023Ateam/resourcegroups/team",
			expectedRawPath: "/planes/radius/%c8%bateam/resourcegroups/team",
			expectedName:    "%c8%bateam",
		},
		{
			name:            "encoded slash stays in one segment",
			target:          "/Planes/radius/a%2Fb/resourceGroups/team",
			expectedPath:    "/planes/radius/a/b/resourcegroups/team",
			expectedRawPath: "/planes/radius/a%2Fb/resourcegroups/team",
			expectedName:    "a%2Fb",
		},
		{
			name:            "encoded keyword",
			target:          "/%50lanes/radius/local/resourceGroups/team",
			expectedPath:    "/planes/radius/local/resourcegroups/team",
			expectedRawPath: "/planes/radius/local/resourcegroups/team",
			expectedName:    "local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath, gotRawPath, gotName string
			calls := 0
			r := chi.NewRouter()
			r.Get("/planes/{planeType}/{planeName}/resourcegroups/{resourceGroupName}", func(w http.ResponseWriter, r *http.Request) {
				calls++
				gotPath, gotRawPath, gotName = r.URL.Path, r.URL.RawPath, chi.URLParam(r, "planeName")
			})

			w := httptest.NewRecorder()
			NormalizePath(r).ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, tt.target, nil))

			require.Equal(t, http.StatusOK, w.Code)
			require.Equal(t, 1, calls)
			require.Equal(t, tt.expectedPath, gotPath)
			require.Equal(t, tt.expectedRawPath, gotRawPath)
			require.Equal(t, tt.expectedName, gotName)
		})
	}
}

func Test_NormalizePath_EscapedFirstOccurrence(t *testing.T) {
	var got *http.Request
	handler := NormalizePath(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x%2FPlanes/Planes/radius/local", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	require.Equal(t, "/x%2FPlanes/planes/radius/local", got.URL.RawPath)
	require.Equal(t, "/x/Planes/planes/radius/local", got.URL.Path)
}
