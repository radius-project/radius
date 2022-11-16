// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
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
		r := mux.NewRouter()
		r.Path("/planes/{planeType}/{planeName}/resourcegroups/{resourceGroup}/providers/Applications.Core/{resourceType}/{resourceName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		r.Path("/planes").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		r.Path("/planes/{planeType}/{planeName}/resourcegroups").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})
		r.Path("/apis/api.ucp.dev/v1alpha3/planes/{planeType}/{planeName}").Methods(http.MethodPost).HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(r.URL.Path))
			})

		handler := NormalizePath(r)

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, tt.armid, nil)
		handler.ServeHTTP(w, req)

		parsed := w.Body.String()
		assert.Equal(t, tt.expected, parsed)
	}
}
