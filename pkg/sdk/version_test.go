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

package sdk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_GetVersion(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
		err  string
	}{
		{name: "release", body: `{"release":"v0.60.2","channel":"0.60"}`, code: 200},
		{name: "old server", code: 404, err: "HTTP 404"},
		{name: "server error", code: 500, err: "HTTP 500"},
		{name: "invalid JSON", body: "<html>", code: 200, err: "invalid Radius version response"},
		{name: "missing release", body: `{"channel":"0.60"}`, code: 200, err: "has no release"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/apis/api.ucp.dev/v1alpha3/version", r.URL.Path)
				require.Equal(t, "application/json", r.Header.Get("Accept"))
				w.WriteHeader(tt.code)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()
			connection, err := NewDirectConnection(server.URL + "/apis/api.ucp.dev/v1alpha3/")
			require.NoError(t, err)
			info, err := GetVersion(t.Context(), connection)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "v0.60.2", info.Release)
		})
	}
}

func Test_GetVersion_Cancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	connection, err := NewDirectConnection("http://127.0.0.1:1")
	require.NoError(t, err)
	_, err = GetVersion(ctx, connection)
	require.ErrorIs(t, err, context.Canceled)
}

func Test_GetVersion_InvalidEndpoint(t *testing.T) {
	_, err := GetVersion(t.Context(), &directConnection{endpoint: "://invalid"})
	require.ErrorContains(t, err, "failed to create")
}

func Test_GetVersion_TransportError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	connection, err := NewDirectConnection(server.URL)
	require.NoError(t, err)
	server.Close()
	_, err = GetVersion(t.Context(), connection)
	require.ErrorContains(t, err, "failed to read Radius version")
}
