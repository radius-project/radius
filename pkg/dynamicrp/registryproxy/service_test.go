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

package registryproxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func startProxy(t *testing.T, target string) (net.Addr, <-chan error, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	listening := make(chan net.Addr, 1)
	service := &Service{
		Options: &Options{
			Enabled:       true,
			ListenAddress: "127.0.0.1:0",
			TargetAddress: target,
		},
		listening: listening,
	}

	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx)
	}()

	select {
	case addr := <-listening:
		return addr, done, cancel
	case err := <-done:
		cancel()
		t.Fatalf("proxy exited before listening: %v", err)
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timed out waiting for proxy to listen")
	}
	return nil, nil, nil
}

func Test_Service_ForwardsHTTPRequests(t *testing.T) {
	t.Parallel()

	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = w.Write([]byte(r.Method + " " + r.URL.Path + " " + string(body)))
	}))
	t.Cleanup(registry.Close)

	addr, done, cancel := startProxy(t, strings.TrimPrefix(registry.URL, "http://"))

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://" + addr.String() + "/v2/")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "GET /v2/ ", string(body))

	resp, err = client.Post("http://"+addr.String()+"/v2/app/blobs/uploads/", "application/octet-stream", strings.NewReader("layer"))
	require.NoError(t, err)
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, "POST /v2/app/blobs/uploads/ layer", string(body))

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for proxy to stop")
	}
}

func Test_Service_ClosesClientWhenTargetUnreachable(t *testing.T) {
	t.Parallel()

	unused, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	target := unused.Addr().String()
	require.NoError(t, unused.Close())

	addr, _, cancel := startProxy(t, target)
	t.Cleanup(cancel)

	conn, err := net.Dial("tcp", addr.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

	_, err = conn.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func Test_Service_StopsOpenConnectionsOnShutdown(t *testing.T) {
	t.Parallel()

	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = upstream.Close() })
	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		// Hold the connection open without responding.
		_, _ = io.Copy(io.Discard, conn)
		_ = conn.Close()
	}()

	addr, done, cancel := startProxy(t, upstream.Addr().String())

	conn, err := net.Dial("tcp", addr.String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Write([]byte("GET /v2/ HTTP/1.1\r\n"))
	require.NoError(t, err)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for proxy to stop with an open connection")
	}
}

func Test_Service_Run_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options *Options
		wantErr string
	}{
		{
			name:    "nil options",
			options: nil,
			wantErr: "registry proxy requires listenAddress and targetAddress",
		},
		{
			name:    "missing listen address",
			options: &Options{TargetAddress: "registry:5000"},
			wantErr: "registry proxy requires listenAddress and targetAddress",
		},
		{
			name:    "missing target address",
			options: &Options{ListenAddress: "127.0.0.1:0"},
			wantErr: "registry proxy requires listenAddress and targetAddress",
		},
		{
			name:    "invalid listen address",
			options: &Options{ListenAddress: "not-an-address", TargetAddress: "registry:5000"},
			wantErr: "registry proxy failed to listen on not-an-address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := (&Service{Options: tt.options}).Run(t.Context())
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
