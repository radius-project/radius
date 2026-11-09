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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

func Test_Run_RadiusReceiver(t *testing.T) {
	t.Parallel()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "radius-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id, err := url.Parse("spiffe://radius.internal/ns/radius-system/sa/applications-rp")
	require.NoError(t, err)
	leafDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{id},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, ca, &key.PublicKey, caKey)
	require.NoError(t, err)

	var requests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER}, PrivateKey: key}}}
	server.StartTLS()
	t.Cleanup(server.Close)

	dir := t.TempDir()
	writePEM(t, dir, "ca.crt", "CERTIFICATE", caDER)
	for _, receiver := range []string{"applications-rp", "dynamic-rp"} {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "tokens", receiver), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "tokens", receiver, "token"), []byte("token-for-"+receiver), 0o600))
	}

	request := func(receiver string) (int, string, string) {
		return run(t, "request", "--radius-receiver", receiver, "--radius-dir", dir, "--radius-namespace", "radius-system", server.URL)
	}

	code, stdout, stderr := request("applications-rp")
	require.Equal(t, ExitOK, code, stderr)
	require.Equal(t, "Bearer token-for-applications-rp"+StatusMarker+"200", stdout)

	// The server's SPIFFE ID is applications-rp's, so a client for dynamic-rp refuses it and
	// sends no request.
	code, stdout, stderr = request("dynamic-rp")
	require.Equal(t, ExitTransport, code)
	require.Equal(t, StatusMarker+"0", stdout)
	require.Contains(t, stderr, "PeerCertificateInvalid")
	require.Equal(t, int32(1), requests.Load())
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
		{name: "receiver without directory", args: []string{"request", "--radius-receiver", "ucp", "--radius-namespace", "ns", "https://x"}, wantErr: "--radius-dir"},
		{name: "receiver with -k", args: []string{"request", "--radius-receiver", "ucp", "--radius-dir", "/d", "--radius-namespace", "ns", "-k", "https://x"}, wantErr: "cannot be combined"},
		{name: "receiver without CA bundle", args: []string{"request", "--radius-receiver", "ucp", "--radius-dir", "/does/not/exist", "--radius-namespace", "ns", "https://x"}, wantErr: "Radius client transport"},
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
