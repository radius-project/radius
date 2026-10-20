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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const ucpServerName = "ucp.radius-system.svc"

type handshakeResult struct {
	serverState tls.ConnectionState
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
		serverDone <- serverResult{state: serverTLS.ConnectionState(), err: err}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	clientTLS := tls.Client(conn, client)
	clientErr := clientTLS.HandshakeContext(ctx)
	_ = conn.Close()
	result := <-serverDone

	return handshakeResult{serverState: result.state, serverErr: result.err, clientErr: clientErr}
}

func clientConfig(src *Source, serverName string) *tls.Config {
	cfg := src.ClientConfig(testNamespace)
	cfg.ServerName = serverName
	return cfg
}

// staticClient returns a client config that presents cert and trusts ca, bypassing Source.
func staticClient(t *testing.T, cert testCert, ca testCA) *tls.Config {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		ServerName:   ucpServerName,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert.tlsCertificate(t)},
	}
}

// staticServer returns a server config that presents cert and requests any client certificate.
func staticServer(t *testing.T, cert testCert) *tls.Config {
	t.Helper()
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert.tlsCertificate(t)},
		ClientAuth:   tls.RequireAnyClientCert,
	}
}

func TestHandshake_Success(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	server := newTestSource(t, ca, "ucp")
	client := newTestSource(t, ca, "controller")

	result := handshake(t, server.ServerConfig(testNamespace), clientConfig(client, ucpServerName))
	require.NoError(t, result.serverErr)
	require.NoError(t, result.clientErr)

	id, err := identityFromChains(result.serverState.VerifiedChains, testNamespace)
	require.NoError(t, err)
	require.Equal(t, "controller.radius-system.svc", id.String())
}

func TestHandshake_NameConstrainedCA(t *testing.T) {
	t.Parallel()
	ca := newConstrainedCA(t, []string{testNamespace + ".svc"})
	server := newTestSource(t, ca, "ucp")
	client := newTestSource(t, ca, "bicep-de")

	result := handshake(t, server.ServerConfig(testNamespace), clientConfig(client, ucpServerName))
	require.NoError(t, result.serverErr)
	require.NoError(t, result.clientErr)

	id, err := identityFromChains(result.serverState.VerifiedChains, testNamespace)
	require.NoError(t, err)
	require.Equal(t, "bicep-de.radius-system.svc", id.String())
}

func TestServerConfig_RejectsClient(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	server := newTestSource(t, ca, "ucp")

	tests := []struct {
		name    string
		wantErr string
		client  func() *tls.Config
	}{
		{name: "wrong CA", wantErr: "unknown authority", client: func() *tls.Config {
			return staticClient(t, newCA(t).issue(t, certOptions{dnsNames: componentSANs("controller")}), ca)
		}},
		{name: "expired", wantErr: "expired", client: func() *tls.Config {
			return staticClient(t, ca.issue(t, certOptions{
				dnsNames:  componentSANs("controller"),
				notBefore: time.Now().Add(-2 * time.Hour),
				notAfter:  time.Now().Add(-time.Hour),
			}), ca)
		}},
		{name: "SAN outside list", wantErr: "is not a Radius component", client: func() *tls.Config {
			return staticClient(t, ca.issue(t, certOptions{dnsNames: []string{"other-app.default.svc"}}), ca)
		}},
		{name: "RSA-1024", wantErr: "RSA key size 1024", client: func() *tls.Config {
			return staticClient(t, ca.issue(t, certOptions{dnsNames: componentSANs("controller"), key: rsaKey(t, 1024)}), ca)
		}},
		{name: "short name SAN", wantErr: "is not a Radius component", client: func() *tls.Config {
			return staticClient(t, ca.issue(t, certOptions{dnsNames: []string{"controller"}}), ca)
		}},
		{name: "more than one SAN", wantErr: "more than one DNS SAN", client: func() *tls.Config {
			return staticClient(t, ca.issue(t, certOptions{dnsNames: []string{canonical("controller"), "controller"}}), ca)
		}},
		{name: "no client certificate", wantErr: "didn't provide a certificate", client: func() *tls.Config {
			cfg := staticClient(t, ca.issue(t, certOptions{dnsNames: componentSANs("controller")}), ca)
			cfg.Certificates = nil
			return cfg
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := handshake(t, server.ServerConfig(testNamespace), tt.client())
			require.ErrorContains(t, result.serverErr, tt.wantErr)
		})
	}
}

func TestClientConfig_RejectsServer(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	client := newTestSource(t, ca, "controller")

	tests := []struct {
		name       string
		server     func() *tls.Config
		serverName string
	}{
		{name: "wrong CA", serverName: ucpServerName, server: func() *tls.Config {
			return staticServer(t, newCA(t).issue(t, certOptions{dnsNames: componentSANs("ucp")}))
		}},
		{name: "hostname mismatch", serverName: canonical("dynamic-rp"), server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}))
		}},
		{name: "expired", serverName: ucpServerName, server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{
				dnsNames:  componentSANs("ucp"),
				notBefore: time.Now().Add(-2 * time.Hour),
				notAfter:  time.Now().Add(-time.Hour),
			}))
		}},
		{name: "two components", serverName: ucpServerName, server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: []string{ucpServerName, canonical("dynamic-rp")}}))
		}},
		{name: "short server name", serverName: "ucp", server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: []string{"ucp"}}))
		}},
		{name: "no server name", serverName: "", server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := handshake(t, tt.server(), clientConfig(client, tt.serverName))
			requireCode(t, result.clientErr, "PeerCertificateInvalid")
		})
	}
}

func TestClientConfig_NoPeerCertificates(t *testing.T) {
	t.Parallel()
	client := newTestSource(t, newCA(t), "controller")

	err := client.ClientConfig(testNamespace).VerifyConnection(tls.ConnectionState{ServerName: ucpServerName})
	requireCode(t, err, "PeerCertificateInvalid")
}

func TestIdentityFromRequest(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	server := newTestSource(t, ca, "ucp")
	client := newTestSource(t, ca, "dynamic-rp")

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := IdentityFromRequest(r, testNamespace)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		ctx := WithIdentity(r.Context(), id)
		fromCtx, ok := IdentityFromContext(ctx)
		if !ok {
			http.Error(w, "missing identity", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(fromCtx.String()))
	}))
	srv.TLS = server.ServerConfig(testNamespace)
	srv.StartTLS()
	t.Cleanup(srv.Close)

	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: clientConfig(client, ucpServerName)}}
	resp, err := httpClient.Get(srv.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "dynamic-rp.radius-system.svc", string(body[:n]))
}

func TestIdentityFromRequest_NoTLS(t *testing.T) {
	t.Parallel()
	_, err := IdentityFromRequest(httptest.NewRequest(http.MethodGet, "/", nil), testNamespace)
	requireCode(t, err, "PeerCertificateInvalid")

	_, ok := IdentityFromContext(t.Context())
	require.False(t, ok)
}
