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

package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type handshakeResult struct {
	serverState tls.ConnectionState
	clientState tls.ConnectionState
	serverErr   error
	clientErr   error
}

// handshake runs a TLS handshake between server and client configs over loopback TCP.
// net.Pipe is not used because it is unbuffered and both sides can block on writes.
func handshake(t *testing.T, server, client *tls.Config) handshakeResult {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	type serverResult struct {
		state tls.ConnectionState
		err   error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- serverResult{err: err}
			return
		}
		defer conn.Close()
		serverTLS := tls.Server(conn, server)
		err = serverTLS.HandshakeContext(ctx)
		if err == nil {
			// Send a byte so the client also reads any TLS 1.3 session ticket.
			_, err = serverTLS.Write([]byte{1})
		}
		serverDone <- serverResult{state: serverTLS.ConnectionState(), err: err}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	clientTLS := tls.Client(conn, client)
	clientErr := clientTLS.HandshakeContext(ctx)
	if clientErr == nil {
		_, clientErr = clientTLS.Read(make([]byte, 1))
	}
	clientState := clientTLS.ConnectionState()
	_ = conn.Close()
	result := <-serverDone

	return handshakeResult{serverState: result.state, clientState: clientState, serverErr: result.err, clientErr: clientErr}
}

// trustClient returns a client config that trusts caPEM and requires receiver's SPIFFE ID.
func trustClient(t *testing.T, caPEM []byte, receiver string) *tls.Config {
	t.Helper()
	return clientConfig(t, newTrustSource(t, caPEM), receiver)
}

// newTrustSource returns a trust-only Source for caPEM.
func newTrustSource(t *testing.T, caPEM []byte) *Source {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, CAFile), caPEM, 0o600))
	src, err := NewTrustSource(dir)
	require.NoError(t, err)
	return src
}

func clientConfig(t *testing.T, src *Source, receiver string) *tls.Config {
	t.Helper()
	cfg, err := src.clientConfig(spiffeID(receiver), nil)
	require.NoError(t, err)
	return cfg
}

// staticServer returns a server config that presents cert.
func staticServer(t *testing.T, cert testCert) *tls.Config {
	t.Helper()
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert.tlsCertificate(t)},
	}
}

func TestHandshake_Success(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	server := newTestSource(t, ca, "ucp")

	var verified []*x509.Certificate
	cfg, err := newTrustSource(t, ca.pem).clientConfig(spiffeID("ucp"), func(chain []*x509.Certificate) { verified = chain })
	require.NoError(t, err)

	result := handshake(t, server.ServerConfig(), cfg)
	require.NoError(t, result.serverErr)
	require.NoError(t, result.clientErr)
	require.Len(t, verified, 2)
	require.Equal(t, ca.cert.Raw, verified[1].Raw)
}

// TestServerConfig_NoClientCertificate checks that the server never requests a client
// certificate, so a client that has one does not send it.
func TestServerConfig_NoClientCertificate(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	server := newTestSource(t, ca, "ucp")
	require.Equal(t, tls.NoClientCert, server.ServerConfig().ClientAuth)

	cfg := trustClient(t, ca.pem, "ucp")
	presented := false
	clientCert := ca.issue(t, certOptions{uris: spiffeURIs("controller")}).tlsCertificate(t)
	cfg.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		presented = true
		return &clientCert, nil
	}

	result := handshake(t, server.ServerConfig(), cfg)
	require.NoError(t, result.serverErr)
	require.NoError(t, result.clientErr)
	require.False(t, presented)
	require.Empty(t, result.serverState.PeerCertificates)
}

func TestServerConfig_OwnCertificateExpired(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	server := newTestSource(t, ca, "ucp")
	server.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	result := handshake(t, server.ServerConfig(), trustClient(t, ca.pem, "ucp"))
	requireCode(t, result.serverErr, "CertificateExpired")
	require.Error(t, result.clientErr)
}

func TestClientConfig_RejectsServer(t *testing.T) {
	t.Parallel()
	ca := newCA(t)

	tests := []struct {
		name    string
		server  func() testCert
		wantErr string
	}{
		{name: "wrong CA", wantErr: "unknown authority", server: func() testCert {
			return newCA(t).issue(t, certOptions{uris: spiffeURIs("ucp")})
		}},
		{name: "wrong SPIFFE ID", wantErr: "is not \"" + spiffeID("ucp") + "\"", server: func() testCert {
			return ca.issue(t, certOptions{uris: spiffeURIs("dynamic-rp")})
		}},
		{name: "DNS name only", wantErr: "no URI SAN", server: func() testCert {
			return ca.issue(t, certOptions{dnsNames: []string{"ucp.radius-system.svc"}})
		}},
		{name: "two URI SANs", wantErr: "more than one URI SAN", server: func() testCert {
			return ca.issue(t, certOptions{uris: []string{spiffeID("ucp"), spiffeID("dynamic-rp")}})
		}},
		{name: "expired", wantErr: "expired", server: func() testCert {
			return ca.issue(t, certOptions{
				uris:      spiffeURIs("ucp"),
				notBefore: time.Now().Add(-2 * time.Hour),
				notAfter:  time.Now().Add(-time.Hour),
			})
		}},
		{name: "client auth only", wantErr: "incompatible key usage", server: func() testCert {
			return ca.issue(t, certOptions{uris: spiffeURIs("ucp"), extKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
		}},
		{name: "RSA-1024", wantErr: "RSA key size 1024", server: func() testCert {
			return ca.issue(t, certOptions{uris: spiffeURIs("ucp"), key: rsaKey(t, 1024)})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := handshake(t, staticServer(t, tt.server()), trustClient(t, ca.pem, "ucp"))
			requireCode(t, result.clientErr, "PeerCertificateInvalid")
			require.ErrorContains(t, result.clientErr, tt.wantErr)
		})
	}
}

func TestClientConfig_NoPeerCertificates(t *testing.T) {
	t.Parallel()
	err := trustClient(t, newCA(t).pem, "ucp").VerifyConnection(tls.ConnectionState{})
	requireCode(t, err, "PeerCertificateInvalid")
}

// TestClientConfig_RotationDuringHandshake rotates the client's CA bundle after the handshake
// starts. The client must still verify the server against the version it started with.
func TestClientConfig_RotationDuringHandshake(t *testing.T) {
	t.Parallel()
	oldCA := newCA(t)
	rotatedCA := newCA(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, CAFile), oldCA.pem, 0o600))
	client, err := NewTrustSource(dir)
	require.NoError(t, err)
	cfg := clientConfig(t, client, "ucp")

	serverCert := oldCA.issue(t, certOptions{uris: spiffeURIs("ucp")}).tlsCertificate(t)
	server := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Rotate after the client has started the handshake and before it sees the server certificate.
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			if err := os.WriteFile(filepath.Join(dir, CAFile), rotatedCA.pem, 0o600); err != nil {
				return nil, err
			}
			if _, err := client.Reload(); err != nil {
				return nil, err
			}
			return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverCert}}, nil
		},
	}

	result := handshake(t, server, cfg)
	require.NoError(t, result.clientErr)
	require.NoError(t, result.serverErr)

	// A new config uses the rotated CA bundle and rejects the old server.
	result = handshake(t, staticServer(t, oldCA.issue(t, certOptions{uris: spiffeURIs("ucp")})), clientConfig(t, client, "ucp"))
	require.ErrorContains(t, result.clientErr, "unknown authority")
}
