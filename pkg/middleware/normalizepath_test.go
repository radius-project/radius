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
	"context"
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

		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		require.NoError(t, err)
		handler.ServeHTTP(w, req)

		parsed := w.Body.String()
		require.Equal(t, tt.expected, parsed)
	}
}
