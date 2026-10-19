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
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
)

const appsRPServerName = "applications-rp.radius-system.svc"

type testServer struct {
	url  string
	logs *observer.ObservedLogs
}

// startServer serves an OK handler as applications-rp in mode, allowing only ucp to call it.
func startServer(t *testing.T, ca testCA, mode authz.Mode) testServer {
	t.Helper()

	dir := t.TempDir()
	writeFiles(t, dir, ca.issue(t, certOptions{dnsNames: componentSANs("applications-rp")}), ca.pem)

	core, logs := observer.New(zapcore.InfoLevel)
	ctx, cancel := context.WithCancel(logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core))))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	options := authz.Options{Mode: mode, CertificateDirectory: dir, Namespace: testNamespace}

	done := make(chan error, 1)
	go func() { done <- Serve(ctx, srv, ln, options, []string{"ucp"}) }()
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})

	return testServer{url: ln.Addr().String(), logs: logs}
}

// client returns an HTTP client that trusts ca and presents cert, if set.
func client(t *testing.T, ca testCA, cert *testCert) *http.Client {
	t.Helper()
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: appsRPServerName, RootCAs: pool}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{cert.tlsCertificate(t)}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 10 * time.Second}
}

func get(c *http.Client, url string) (int, error) {
	resp, err := c.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// requireLogged waits for a log entry with the mode's denial field, code, and a reason
// or caller containing want.
func requireLogged(t *testing.T, logs *observer.ObservedLogs, mode authz.Mode, code, want string) {
	t.Helper()
	field := authz.LogFieldWouldDeny
	if mode == authz.ModeEnforce {
		field = authz.LogFieldDenied
	}
	require.Eventually(t, func() bool {
		for _, entry := range logs.All() {
			m := entry.ContextMap()
			if m[field] != true || m[authz.LogFieldCode] != code {
				continue
			}
			detail, _ := m[LogFieldReason].(string)
			caller, _ := m[LogFieldCaller].(string)
			if strings.Contains(detail, want) || strings.Contains(caller, want) {
				return true
			}
		}
		return false
	}, 5*time.Second, 10*time.Millisecond, "no %s log with code %s containing %q: %v", field, code, want, logs.All())
}

type peerCase struct {
	name string
	// cert is the client certificate; nil presents none. plain sends HTTP instead of TLS.
	cert  *testCert
	plain bool
	code  string
	want  string
}

func peerCases(t *testing.T, ca testCA) (allowed testCert, cases []peerCase) {
	t.Helper()
	otherCA := newCA(t)
	ucp := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	wrongCA := otherCA.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	expired := ca.issue(t, certOptions{dnsNames: componentSANs("ucp"), notBefore: time.Now().Add(-2 * time.Hour), notAfter: time.Now().Add(-time.Hour)})
	rsa1024 := ca.issue(t, certOptions{dnsNames: componentSANs("dynamic-rp"), key: rsaKey(t, 1024)})
	dynamicRP := ca.issue(t, certOptions{dnsNames: componentSANs("dynamic-rp"), key: ecKey(t, elliptic.P384())})

	return ucp, []peerCase{
		{name: "plain HTTP", plain: true, code: authz.CodePeerCertificateInvalid, want: "plain HTTP"},
		{name: "no client certificate", code: authz.CodePeerCertificateInvalid, want: "certificate"},
		{name: "wrong CA", cert: &wrongCA, code: authz.CodePeerCertificateInvalid, want: "unknown authority"},
		{name: "expired", cert: &expired, code: authz.CodePeerCertificateInvalid, want: "expired"},
		{name: "RSA-1024", cert: &rsa1024, code: authz.CodePeerCertificateInvalid, want: "RSA key size 1024"},
		{name: "caller not allowed", cert: &dynamicRP, code: authz.CodeAuthorizationFailed, want: "dynamic-rp.radius-system.svc"},
	}
}

func TestServe_Enforce(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)
	ucp, cases := peerCases(t, ca)

	status, err := get(client(t, ca, &ucp), "https://"+srv.url)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.plain {
				status, err := get(http.DefaultClient, "http://"+srv.url)
				require.NoError(t, err)
				require.Equal(t, http.StatusBadRequest, status)
			} else {
				status, err := get(client(t, ca, tc.cert), "https://"+srv.url)
				if tc.code == authz.CodeAuthorizationFailed {
					require.NoError(t, err)
					require.Equal(t, http.StatusForbidden, status)
				} else {
					require.Error(t, err)
				}
			}
			requireLogged(t, srv.logs, authz.ModeEnforce, tc.code, tc.want)
		})
	}
}

func TestServe_DryRun(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeDryRun)
	ucp, cases := peerCases(t, ca)

	status, err := get(client(t, ca, &ucp), "https://"+srv.url)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := "https://" + srv.url
			c := client(t, ca, tc.cert)
			if tc.plain {
				url = "http://" + srv.url
				c = http.DefaultClient
			}
			status, err := get(c, url)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, status)
			requireLogged(t, srv.logs, authz.ModeDryRun, tc.code, tc.want)
		})
	}

	for _, entry := range srv.logs.All() {
		require.NotContains(t, entry.ContextMap(), authz.LogFieldDenied)
	}
}

func TestServe_Off(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeOff)
	ucp := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})

	status, err := get(http.DefaultClient, "http://"+srv.url)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	_, err = get(client(t, ca, &ucp), "https://"+srv.url)
	require.Error(t, err)
	require.Empty(t, srv.logs.All())
}

func TestServe_EnforceDeniedResponse(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)
	controller := ca.issue(t, certOptions{dnsNames: componentSANs("controller")})

	resp, err := client(t, ca, &controller).Get("https://" + srv.url + "/apis/test")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	require.Contains(t, string(body), v1.CodeAuthorizationFailed)
	require.Contains(t, string(body), "/apis/test")
}

func TestServe_ConfigErrors(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		options authz.Options
		want    string
	}{
		{name: "missing directory", options: authz.Options{Mode: authz.ModeEnforce, Namespace: testNamespace}, want: "requires certificateDirectory"},
		{name: "missing namespace", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: t.TempDir()}, want: "requires certificateDirectory and namespace"},
		{name: "no certificate files", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: t.TempDir(), Namespace: testNamespace}, want: "failed to load mTLS certificate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			err = Serve(t.Context(), &http.Server{}, ln, tc.options, nil)
			require.ErrorContains(t, err, tc.want)

			_, err = ln.Accept()
			require.True(t, errors.Is(err, net.ErrClosed), "listener should be closed: %v", err)
		})
	}
}

func TestRequestClientCertConfig_UsesOneSnapshot(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	dir := t.TempDir()
	writeFiles(t, dir, ca.issue(t, certOptions{dnsNames: componentSANs("applications-rp")}), ca.pem)
	src, err := NewSource(dir)
	require.NoError(t, err)

	conn := &peekedConn{}
	cfg, err := src.requestClientCertConfig(testNamespace).GetConfigForClient(&tls.ClientHelloInfo{Conn: conn})
	require.NoError(t, err)

	// Rotate to a new CA after the handshake picked its material. The client is still
	// checked against the CA bundle that matches the certificate the server presented.
	rotated := newCA(t)
	rotatedDir := t.TempDir()
	writeFiles(t, rotatedDir, rotated.issue(t, certOptions{dnsNames: componentSANs("applications-rp")}), rotated.pem)
	next, err := NewSource(rotatedDir)
	require.NoError(t, err)
	src.current.Store(next.current.Load())

	ucp := ca.issue(t, certOptions{dnsNames: componentSANs("ucp")})
	require.NoError(t, cfg.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{ucp.cert}}))
	require.True(t, conn.checked)
	require.NoError(t, conn.peerErr)
	require.Equal(t, "ucp", conn.identity.Component)
}
