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

package rogueclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Run(t.Context(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func Test_Run_SendsRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPut || r.Header.Get("x-remote-user") != "admin" ||
			r.Header.Get("Content-Type") != "application/json" || string(body) != `{"a":"$b"}` {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("denied\n"))
	}))
	t.Cleanup(server.Close)

	code, stdout, stderr := run(t, "request",
		"-X", "PUT",
		"-H", "x-remote-user: admin",
		"-H", "Content-Type: application/json",
		"--data", `{"a":"$b"}`,
		server.URL)

	require.Equal(t, ExitOK, code, stderr)
	require.Equal(t, "denied\n"+StatusMarker+"403", stdout)
}

func Test_Run_DefaultsToGET(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(server.Close)

	code, stdout, _ := run(t, "request", server.URL)

	require.Equal(t, ExitOK, code)
	require.Equal(t, "ok"+StatusMarker+"200", stdout)
}

func Test_Run_TransportFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.NotFoundHandler())
	url := server.URL
	server.Close()

	code, stdout, stderr := run(t, "request", url)

	require.Equal(t, ExitTransport, code)
	require.Equal(t, StatusMarker+"0", stdout)
	require.NotEmpty(t, stderr)
}

func Test_Run_TLS(t *testing.T) {
	t.Parallel()

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) == 0 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("client cert presented"))
	}))
	server.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	server.StartTLS()
	t.Cleanup(server.Close)

	// The httptest server certificate doubles as the CA and the client certificate.
	dir := t.TempDir()
	cert := server.TLS.Certificates[0]
	certFile := writePEM(t, dir, "tls.crt", "CERTIFICATE", cert.Certificate[0])
	keyDER, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	require.NoError(t, err)
	keyFile := writePEM(t, dir, "tls.key", "PRIVATE KEY", keyDER)

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{name: "untrusted server fails", wantCode: ExitTransport, wantOut: StatusMarker + "0"},
		{name: "insecure skips verification", args: []string{"-k"}, wantCode: ExitOK, wantOut: StatusMarker + "401"},
		{name: "CA without client certificate", args: []string{"--cacert", certFile}, wantCode: ExitOK, wantOut: StatusMarker + "401"},
		{
			name:     "CA and client certificate",
			args:     []string{"--cacert", certFile, "--cert", certFile, "--key", keyFile},
			wantCode: ExitOK,
			wantOut:  "client cert presented" + StatusMarker + "200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{"request"}, tt.args...), server.URL)
			code, stdout, stderr := run(t, args...)
			require.Equal(t, tt.wantCode, code, stderr)
			require.Equal(t, tt.wantOut, stdout)
		})
	}
}

func Test_Run_UsageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no command", args: nil, wantErr: "usage"},
		{name: "unknown command", args: []string{"bogus"}, wantErr: "unknown command"},
		{name: "missing URL", args: []string{"request"}, wantErr: "exactly one URL"},
		{name: "unknown flag", args: []string{"request", "--bogus", "http://x"}, wantErr: "bogus"},
		{name: "malformed header", args: []string{"request", "-H", "nocolon", "http://x"}, wantErr: "invalid header"},
		{name: "cert without key", args: []string{"request", "--cert", "/c", "http://x"}, wantErr: "--cert and --key"},
		{name: "missing CA file", args: []string{"request", "--cacert", "/does/not/exist", "http://x"}, wantErr: "reading CA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := run(t, tt.args...)
			require.Equal(t, ExitUsage, code)
			require.Empty(t, stdout)
			require.Contains(t, stderr, tt.wantErr)
		})
	}
}

func Test_Run_IdleStopsWhenCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var stdout, stderr bytes.Buffer
	require.Equal(t, ExitOK, Run(ctx, []string{"idle"}, &stdout, &stderr))
}

func writePEM(t *testing.T, dir, name, blockType string, der []byte) string {
	t.Helper()
	file := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600))
	return file
}
