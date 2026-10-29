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

func clientConfig(t *testing.T, src *Source, serverName string) *tls.Config {
	t.Helper()
	cfg, err := src.ClientConfig(testNamespace)
	require.NoError(t, err)
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

	result := handshake(t, server.ServerConfig(testNamespace), clientConfig(t, client, ucpServerName))
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

	result := handshake(t, server.ServerConfig(testNamespace), clientConfig(t, client, ucpServerName))
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

	// The standard verifier rejects the chain, dates, and name; VerifyConnection rejects the identity.
	tests := []struct {
		name       string
		server     func() *tls.Config
		serverName string
		wantErr    string
		wantCode   string
	}{
		{name: "wrong CA", serverName: ucpServerName, wantErr: "unknown authority", server: func() *tls.Config {
			return staticServer(t, newCA(t).issue(t, certOptions{dnsNames: componentSANs("ucp")}))
		}},
		{name: "hostname mismatch", serverName: canonical("dynamic-rp"), wantErr: "not " + canonical("dynamic-rp"), server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}))
		}},
		{name: "expired", serverName: ucpServerName, wantErr: "expired", server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{
				dnsNames:  componentSANs("ucp"),
				notBefore: time.Now().Add(-2 * time.Hour),
				notAfter:  time.Now().Add(-time.Hour),
			}))
		}},
		{name: "no server name", serverName: "", wantErr: "ServerName", server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp")}))
		}},
		{name: "two components", serverName: ucpServerName, wantCode: "PeerCertificateInvalid", server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: []string{ucpServerName, canonical("dynamic-rp")}}))
		}},
		{name: "short server name", serverName: "ucp", wantCode: "PeerCertificateInvalid", server: func() *tls.Config {
			return staticServer(t, ca.issue(t, certOptions{dnsNames: []string{"ucp"}}))
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := handshake(t, tt.server(), clientConfig(t, client, tt.serverName))
			if tt.wantCode != "" {
				requireCode(t, result.clientErr, tt.wantCode)
				return
			}
			require.ErrorContains(t, result.clientErr, tt.wantErr)
		})
	}
}

// TestClientConfig_OwnCertificateExpired checks that a config whose certificate expires
// after it was created fails closed, even when the server does not request a certificate.
func TestClientConfig_OwnCertificateExpired(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	client := newTestSource(t, ca, "controller")
	cfg := clientConfig(t, client, ucpServerName)
	client.now = func() time.Time { return time.Now().Add(2 * time.Hour) }

	server := staticServer(t, ca.issue(t, certOptions{dnsNames: componentSANs("ucp"), notAfter: time.Now().Add(3 * time.Hour)}))
	server.ClientAuth = tls.NoClientCert

	result := handshake(t, server, cfg)
	requireCode(t, result.clientErr, "CertificateExpired")
}

func TestClientConfig_NoPeerCertificates(t *testing.T) {
	t.Parallel()
	client := newTestSource(t, newCA(t), "controller")

	err := clientConfig(t, client, ucpServerName).VerifyConnection(tls.ConnectionState{ServerName: ucpServerName})
	requireCode(t, err, "PeerCertificateInvalid")
}

// TestServerConfig_NoResumption checks that the server never resumes a session, so every
// connection verifies the client against the current CA bundle, including after a CA is
// removed from the bundle.
func TestServerConfig_NoResumption(t *testing.T) {
	t.Parallel()
	for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		t.Run(tls.VersionName(version), func(t *testing.T) {
			t.Parallel()
			retiredCA, keptCA := newCA(t), newCA(t)
			bothCAs := append(append([]byte{}, retiredCA.pem...), keptCA.pem...)
			serverCert := keptCA.issue(t, certOptions{dnsNames: componentSANs("ucp")})
			dir := newKubeletDir(t, serverCert, bothCAs)
			server, err := NewSource(dir.root)
			require.NoError(t, err)

			clientDir := t.TempDir()
			writeFiles(t, clientDir, retiredCA.issue(t, certOptions{dnsNames: componentSANs("controller")}), bothCAs)
			client, err := NewSource(clientDir)
			require.NoError(t, err)
			cfg := clientConfig(t, client, ucpServerName)
			cfg.MaxVersion = version
			cfg.ClientSessionCache = tls.NewLRUClientSessionCache(1)
			serverConfig := server.ServerConfig(testNamespace)

			for range 2 {
				result := handshake(t, serverConfig, cfg)
				require.NoError(t, result.serverErr)
				require.NoError(t, result.clientErr)
				require.False(t, result.serverState.DidResume)
			}

			dir.update(t, serverCert, keptCA.pem)
			_, err = server.Reload()
			require.NoError(t, err)

			result := handshake(t, serverConfig, cfg)
			require.ErrorContains(t, result.serverErr, "unknown authority")
		})
	}
}

// TestClientConfig_RotationDuringHandshake rotates the client's certificate and CA to a
// new CA after the handshake starts. The client must still verify the server and present
// its certificate from the version it started with.
func TestClientConfig_RotationDuringHandshake(t *testing.T) {
	t.Parallel()
	oldCA := newCA(t)
	rotatedCA := newCA(t)
	dir := newKubeletDir(t, oldCA.issue(t, certOptions{dnsNames: componentSANs("controller")}), oldCA.pem)
	client, err := NewSource(dir.root)
	require.NoError(t, err)
	cfg := clientConfig(t, client, ucpServerName)
	rotate := dir.stage(t, rotatedCA.issue(t, certOptions{dnsNames: componentSANs("controller")}), rotatedCA.pem)

	pool := x509.NewCertPool()
	pool.AddCert(oldCA.cert)
	serverCert := oldCA.issue(t, certOptions{dnsNames: componentSANs("ucp")}).tlsCertificate(t)
	server := &tls.Config{
		MinVersion: tls.VersionTLS12,
		// Rotate after the client has started the handshake and before it sees the server certificate.
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			if err := rotate(); err != nil {
				return nil, err
			}
			if _, err := client.Reload(); err != nil {
				return nil, err
			}
			return &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{serverCert},
				ClientAuth:   tls.RequireAndVerifyClientCert,
				ClientCAs:    pool,
			}, nil
		},
	}

	result := handshake(t, server, cfg)
	require.NoError(t, result.clientErr)
	require.NoError(t, result.serverErr)
	id, err := identityFromChains(result.serverState.VerifiedChains, testNamespace)
	require.NoError(t, err)
	require.Equal(t, "controller.radius-system.svc", id.String())

	// A new config uses the rotated version.
	m, err := client.load()
	require.NoError(t, err)
	require.Same(t, m.cert, mustClientCert(t, clientConfig(t, client, ucpServerName)))
}

func mustClientCert(t *testing.T, cfg *tls.Config) *tls.Certificate {
	t.Helper()
	cert, err := cfg.GetClientCertificate(nil)
	require.NoError(t, err)
	return cert
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

	httpClient := &http.Client{Transport: &http.Transport{TLSClientConfig: clientConfig(t, client, ucpServerName)}}
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
