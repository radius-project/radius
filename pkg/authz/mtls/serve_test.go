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
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/token"
)

const (
	// testPollInterval is how often test servers reload their certificate files.
	testPollInterval = 20 * time.Millisecond

	testIssuer = "https://kubernetes.default.svc.cluster.local"
	testKeyID  = "test-key"
)

// signingKey signs the test ServiceAccount tokens.
var signingKey = func() *ecdsa.PrivateKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return k
}()

// staticKeys is a token.KeySource that returns signingKey's public key.
type staticKeys struct{}

func (staticKeys) Fetch(context.Context) (token.KeySet, error) {
	return token.KeySet{Issuer: testIssuer, Keys: map[string]crypto.PublicKey{testKeyID: &signingKey.PublicKey}}, nil
}

// testClock is a settable clock.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// mintToken returns a projected ServiceAccount token for account with audience, as the
// kubelet issues it.
func mintToken(t *testing.T, account, audience string) string {
	t.Helper()
	return mintTokenWithKey(t, account, audience, signingKey)
}

func mintTokenWithKey(t *testing.T, account, audience string, key *ecdsa.PrivateKey) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": testIssuer,
		"aud": []string{audience},
		"sub": "system:serviceaccount:" + testNamespace + ":" + account,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
		"kubernetes.io": map[string]any{
			"namespace":      testNamespace,
			"serviceaccount": map[string]any{"name": account, "uid": "1234"},
		},
	})
	tok.Header["kid"] = testKeyID
	signed, err := tok.SignedString(key)
	require.NoError(t, err)
	return signed
}

type testServer struct {
	url   string
	dir   string
	logs  *observer.ObservedLogs
	clock *testClock
	// seen receives the Authorization header and identity of each request the handler served.
	seen chan seenRequest
}

type seenRequest struct {
	authorization string
	identity      authz.Identity
	hasIdentity   bool
}

// startServer serves applications-rp in mode, allowing only ucp to call it, and records each
// request the handler sees.
func startServer(t *testing.T, ca testCA, mode authz.Mode) testServer {
	t.Helper()
	seen := make(chan seenRequest, 16)
	srv := startServerWithHandler(t, ca, mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := authz.IdentityFromContext(r.Context())
		seen <- seenRequest{authorization: r.Header.Get("Authorization"), identity: id, hasIdentity: ok}
		w.WriteHeader(http.StatusOK)
	}))
	srv.seen = seen
	return srv
}

// startServerWithHandler is startServer with a custom handler; nil uses http.DefaultServeMux.
func startServerWithHandler(t *testing.T, ca testCA, mode authz.Mode, handler http.Handler) testServer {
	t.Helper()

	dir := t.TempDir()
	writeFiles(t, dir, ca.issue(t, certOptions{uris: spiffeURIs("applications-rp")}), ca.pem)

	core, logs := observer.New(zapcore.InfoLevel)
	ctx, cancel := context.WithCancel(logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core))))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{
		Handler:     handler,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	options := authz.Options{Mode: mode, CertificateDirectory: dir, Namespace: testNamespace}
	clock := &testClock{now: time.Now()}
	config := serveConfig{pollInterval: testPollInterval, tokenNow: clock.Now}
	receiver := Receiver{Name: "applications-rp", AllowedCallers: []string{"ucp"}, KeySource: staticKeys{}}

	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, ln, options, receiver, config) }()
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})

	return testServer{url: ln.Addr().String(), dir: dir, logs: logs, clock: clock}
}

// client returns an HTTP client that verifies the applications-rp certificate from ca and
// presents no client certificate.
func client(t *testing.T, ca testCA) *http.Client {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: trustClient(t, ca.pem, "applications-rp")}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 10 * time.Second}
}

// send sends GET url with authorization, if set, and returns the status code.
func send(t *testing.T, c *http.Client, url, authorization string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := c.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
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

type requestCase struct {
	name          string
	authorization string
	// status is the enforce-mode response.
	status int
	code   string
	want   string
}

func requestCases(t *testing.T) []requestCase {
	t.Helper()
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return []requestCase{
		{name: "no token", status: http.StatusUnauthorized, code: v1.CodeInvalidAuthenticationInfo, want: "missing bearer token"},
		{name: "not a bearer token", authorization: "Basic dXNlcjpwYXNz", status: http.StatusUnauthorized, code: v1.CodeInvalidAuthenticationInfo, want: "not a bearer token"},
		{name: "wrong audience", authorization: "Bearer " + mintToken(t, "ucp", "dynamic-rp"), status: http.StatusUnauthorized, code: v1.CodeInvalidAuthenticationInfo, want: "token is not valid"},
		{name: "unknown signing key", authorization: "Bearer " + mintTokenWithKey(t, "ucp", "applications-rp", otherKey), status: http.StatusUnauthorized, code: v1.CodeInvalidAuthenticationInfo, want: "token is not valid"},
		{name: "not a Radius ServiceAccount", authorization: "Bearer " + mintToken(t, "default", "applications-rp"), status: http.StatusUnauthorized, code: v1.CodeInvalidAuthenticationInfo, want: "default"},
		{name: "caller not allowed", authorization: "Bearer " + mintToken(t, "controller", "applications-rp"), status: http.StatusForbidden, code: authz.CodeAuthorizationFailed, want: "controller.radius-system.svc"},
	}
}

func TestServe_ValidToken(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			ca := newCA(t)
			srv := startServer(t, ca, mode)

			status := send(t, client(t, ca), "https://"+srv.url, "Bearer "+mintToken(t, "ucp", "applications-rp"))
			require.Equal(t, http.StatusOK, status)

			seen := <-srv.seen
			require.True(t, seen.hasIdentity)
			require.Equal(t, "ucp.radius-system.svc", seen.identity.String())
			require.Empty(t, seen.authorization, "the token must not reach the handler")
			require.Empty(t, srv.logs.All())
		})
	}
}

func TestServe_Enforce(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)
	c := client(t, ca)

	for _, tc := range requestCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.status, send(t, c, "https://"+srv.url, tc.authorization))
			requireLogged(t, srv.logs, authz.ModeEnforce, tc.code, tc.want)
		})
	}

	t.Run("plain HTTP", func(t *testing.T) {
		require.Equal(t, http.StatusBadRequest, send(t, http.DefaultClient, "http://"+srv.url, "Bearer "+mintToken(t, "ucp", "applications-rp")))
		requireLogged(t, srv.logs, authz.ModeEnforce, v1.CodeInvalidAuthenticationInfo, "plain HTTP")
	})

	require.Empty(t, srv.seen, "denied requests must not reach the handler")
}

func TestServe_DryRun(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeDryRun)
	c := client(t, ca)

	for _, tc := range requestCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, http.StatusOK, send(t, c, "https://"+srv.url, tc.authorization))
			seen := <-srv.seen
			require.Empty(t, seen.authorization, "the token must not reach the handler")
			require.False(t, seen.hasIdentity, "a request that would be denied has no identity")
			requireLogged(t, srv.logs, authz.ModeDryRun, tc.code, tc.want)
		})
	}

	t.Run("plain HTTP", func(t *testing.T) {
		require.Equal(t, http.StatusOK, send(t, http.DefaultClient, "http://"+srv.url, ""))
		<-srv.seen
		requireLogged(t, srv.logs, authz.ModeDryRun, v1.CodeInvalidAuthenticationInfo, "plain HTTP")
	})

	for _, entry := range srv.logs.All() {
		require.NotContains(t, entry.ContextMap(), authz.LogFieldDenied)
	}
}

// TestServe_StaleSigningKeys checks that a server whose signing keys could not be refreshed
// fails closed with AuthorizationUnavailable instead of trusting old keys.
func TestServe_StaleSigningKeys(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			ca := newCA(t)
			srv := startServer(t, ca, mode)
			c := client(t, ca)
			tok := "Bearer " + mintToken(t, "ucp", "applications-rp")
			require.Equal(t, http.StatusOK, send(t, c, "https://"+srv.url, tok))
			<-srv.seen
			srv.clock.Advance(token.DefaultMaxKeyAge)
			for _, authorization := range []string{tok, "", "Basic invalid"} {
				require.Equal(t, http.StatusServiceUnavailable, send(t, c, "https://"+srv.url, authorization))
			}
			require.Empty(t, srv.seen, "unavailable verification must not reach the handler, even in dryRun")
			requireLogged(t, srv.logs, authz.ModeEnforce, v1.CodeAuthorizationUnavailable, "signing keys")
		})
	}
}

// TestServe_IgnoresClientCertificate checks that a client certificate does not authenticate
// a request.
func TestServe_IgnoresClientCertificate(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)

	cfg := trustClient(t, ca.pem, "applications-rp")
	cfg.Certificates = []tls.Certificate{ca.issue(t, certOptions{uris: spiffeURIs("ucp"), dnsNames: []string{"ucp.radius-system.svc"}}).tlsCertificate(t)}
	transport := &http.Transport{TLSClientConfig: cfg}
	t.Cleanup(transport.CloseIdleConnections)

	require.Equal(t, http.StatusUnauthorized, send(t, &http.Client{Transport: transport}, "https://"+srv.url, ""))
}

// TestServe_ReloadsServerCertificate rotates the server certificate and CA and checks that new
// connections get the new certificate.
func TestServe_ReloadsServerCertificate(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)
	tok := "Bearer " + mintToken(t, "ucp", "applications-rp")
	require.Equal(t, http.StatusOK, send(t, client(t, ca), "https://"+srv.url, tok))
	<-srv.seen

	rotated := newCA(t)
	writeFiles(t, srv.dir, rotated.issue(t, certOptions{uris: spiffeURIs("applications-rp")}), rotated.pem)
	require.Eventually(t, func() bool {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+srv.url, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", tok)
		transport := &http.Transport{TLSClientConfig: trustClient(t, rotated.pem, "applications-rp")}
		defer transport.CloseIdleConnections()
		resp, err := (&http.Client{Transport: transport}).Do(req)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 5*time.Second, 20*time.Millisecond)
}

func TestServe_Off(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeOff)

	require.Equal(t, http.StatusOK, send(t, http.DefaultClient, "http://"+srv.url, ""))
	seen := <-srv.seen
	require.False(t, seen.hasIdentity)

	_, err := client(t, ca).Get("https://" + srv.url)
	require.Error(t, err)
	require.Empty(t, srv.logs.All())
}

// registerDefaultServeMux registers the test handler once, so -count > 1 doesn't panic.
var registerDefaultServeMux sync.Once

func TestServe_NilHandlerUsesDefaultServeMux(t *testing.T) {
	t.Parallel()
	const path = "/mtls-test/default-serve-mux"
	registerDefaultServeMux.Do(func() {
		http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		})
	})

	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			ca := newCA(t)
			srv := startServerWithHandler(t, ca, mode, nil)
			status := send(t, client(t, ca), "https://"+srv.url+path, "Bearer "+mintToken(t, "ucp", "applications-rp"))
			require.Equal(t, http.StatusTeapot, status)
		})
	}
}

func TestServe_EnforceDeniedResponse(t *testing.T) {
	t.Parallel()
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+srv.url+"/apis/test", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+mintToken(t, "controller", "applications-rp"))
	resp, err := client(t, ca).Do(req)
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
	ca := newCA(t)
	certDir := t.TempDir()
	leaf := ca.issue(t, certOptions{uris: spiffeURIs("applications-rp")})
	writeFiles(t, certDir, leaf, ca.pem)
	// csiDir is laid out like a csi-driver-spiffe volume without a source CA bundle: no ca.crt.
	csiDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(csiDir, CertFile), leaf.certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(csiDir, KeyFile), leaf.keyPEM, 0o600))
	receiver := Receiver{Name: "applications-rp"}

	for _, tc := range []struct {
		name     string
		options  authz.Options
		receiver Receiver
		want     string
	}{
		{name: "missing directory", options: authz.Options{Mode: authz.ModeEnforce, Namespace: testNamespace}, receiver: receiver, want: "requires certificateDirectory"},
		{name: "missing namespace", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir}, receiver: receiver, want: "requires certificateDirectory and namespace"},
		{name: "missing receiver", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir, Namespace: testNamespace}, want: "receiver name is required"},
		{name: "no certificate files", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: t.TempDir(), Namespace: testNamespace}, receiver: receiver, want: "failed to load server certificate"},
		{name: "CSI volume without CA directory", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: csiDir, Namespace: testNamespace}, receiver: receiver, want: "failed to read ca.crt"},
		{name: "no CA bundle in CA directory", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir, CADirectory: t.TempDir(), Namespace: testNamespace}, receiver: receiver, want: "failed to read ca.crt"},
		{name: "CA bundle from CA directory", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: csiDir, CADirectory: certDir, Namespace: testNamespace}, receiver: receiver, want: "in-cluster config"},
		{name: "not in a cluster", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir, Namespace: testNamespace}, receiver: receiver, want: "in-cluster config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			err = Serve(t.Context(), &http.Server{}, ln, tc.options, tc.receiver)
			require.ErrorContains(t, err, tc.want)

			_, err = ln.Accept()
			require.True(t, errors.Is(err, net.ErrClosed), "listener should be closed: %v", err)
		})
	}
}

// writeToken writes tok as the projected token for receiver under dir.
func writeToken(t *testing.T, dir, receiver, tok string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, receiver), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, receiver, TokenFile), []byte(tok+"\n"), 0o600))
}
