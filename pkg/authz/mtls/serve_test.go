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
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"k8s.io/client-go/rest"

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

// staticKeys is a token.KeySource that returns signingKey's public key until it is made
// unavailable.
type staticKeys struct {
	unavailable atomic.Bool
}

func (k *staticKeys) Fetch(context.Context) (token.KeySet, error) {
	if k.unavailable.Load() {
		return token.KeySet{}, errors.New("signing keys are unavailable")
	}
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
	keys  *staticKeys
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
	return startRecordingServer(t, ca, mode, serverSetup{})
}

// startRecordingServer is startServer with setup.
func startRecordingServer(t *testing.T, ca testCA, mode authz.Mode, setup serverSetup) testServer {
	t.Helper()
	seen := make(chan seenRequest, 16)
	srv := startServerWithSetup(t, ca, mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := authz.IdentityFromContext(r.Context())
		seen <- seenRequest{authorization: r.Header.Get("Authorization"), identity: id, hasIdentity: ok}
		w.WriteHeader(http.StatusOK)
	}), setup)
	srv.seen = seen
	return srv
}

// startServerWithHandler is startServer with a custom handler; nil uses http.DefaultServeMux.
func startServerWithHandler(t *testing.T, ca testCA, mode authz.Mode, handler http.Handler) testServer {
	return startServerWithProvider(t, ca, mode, handler, authz.TLSProviderRadius)
}

func startServerWithProvider(t *testing.T, ca testCA, mode authz.Mode, handler http.Handler, provider authz.TLSProvider) testServer {
	t.Helper()
	return startServerWithSetup(t, ca, mode, handler, serverSetup{provider: provider})
}

// serverSetup overrides the defaults of a test server.
type serverSetup struct {
	provider authz.TLSProvider
	// leaf is the serving certificate. Nil issues one from the CA that is valid for an hour.
	leaf *testCert
	// certNow is the serving certificate's clock.
	certNow func() time.Time
}

func startServerWithSetup(t *testing.T, ca testCA, mode authz.Mode, handler http.Handler, setup serverSetup) testServer {
	t.Helper()

	dir := t.TempDir()
	provider := setup.provider
	if provider == "" {
		provider = authz.TLSProviderRadius
	}
	if provider == authz.TLSProviderRadius {
		leaf := setup.leaf
		if leaf == nil {
			issued := ca.issue(t, certOptions{uris: spiffeURIs("applications-rp")})
			leaf = &issued
		}
		writeFiles(t, dir, *leaf, ca.pem)
	}

	core, logs := observer.New(zapcore.InfoLevel)
	ctx, cancel := context.WithCancel(logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core))))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := &http.Server{
		Handler:     handler,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	options := authz.Options{Mode: mode, TLSProvider: provider, Namespace: testNamespace}
	if provider == authz.TLSProviderRadius {
		options.CertificateDirectory = dir
	}
	clock := &testClock{now: time.Now()}
	keys := &staticKeys{}
	config := serveConfig{pollInterval: testPollInterval, tokenNow: clock.Now, certNow: setup.certNow}
	receiver := Receiver{Name: "applications-rp", AllowedCallers: []string{"ucp"}, KeySource: keys}

	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, ln, options, receiver, config) }()
	t.Cleanup(func() {
		cancel()
		_ = srv.Close()
		require.ErrorIs(t, <-done, http.ErrServerClosed)
	})

	return testServer{url: ln.Addr().String(), dir: dir, logs: logs, clock: clock, keys: keys}
}

func TestServe_ExternalTokenAuthorization(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			seen := make(chan seenRequest, 16)
			srv := startServerWithProvider(t, testCA{}, mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, ok := authz.IdentityFromContext(r.Context())
				seen <- seenRequest{authorization: r.Header.Get("Authorization"), identity: id, hasIdentity: ok}
			}), authz.TLSProviderExternal)
			address := "http://" + srv.url
			c := &http.Client{Timeout: 5 * time.Second}
			require.Equal(t, http.StatusOK, send(t, c, address, "Bearer "+mintToken(t, "ucp", "applications-rp")))
			valid := <-seen
			require.True(t, valid.hasIdentity)
			require.Equal(t, "ucp.radius-system.svc", valid.identity.String())
			require.Empty(t, valid.authorization)

			for _, tc := range requestCases(t) {
				t.Run(tc.name, func(t *testing.T) {
					want := tc.status
					if mode == authz.ModeDryRun {
						want = http.StatusOK
					}
					require.Equal(t, want, send(t, c, address, tc.authorization))
					if mode == authz.ModeDryRun {
						got := <-seen
						require.False(t, got.hasIdentity)
						require.Empty(t, got.authorization)
					}
					requireLogged(t, srv.logs, mode, tc.code, tc.want)
				})
			}
			srv.keys.unavailable.Store(true)
			srv.clock.Advance(token.DefaultMaxKeyAge)
			for _, authorization := range []string{"", "Bearer malformed", "Bearer " + mintToken(t, "ucp", "applications-rp")} {
				require.Equal(t, http.StatusServiceUnavailable, send(t, c, address, authorization))
			}
			require.Empty(t, seen, "stale-key requests must not reach handlers in either mode")
		})
	}
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
		// A valid token sent in plain text is served, but it does not identify the caller.
		require.Equal(t, http.StatusOK, send(t, http.DefaultClient, "http://"+srv.url, "Bearer "+mintToken(t, "ucp", "applications-rp")))
		seen := <-srv.seen
		require.Empty(t, seen.authorization, "the token must not reach the handler")
		require.False(t, seen.hasIdentity, "a plain HTTP request has no identity")
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
	ca := newCA(t)
	srv := startServer(t, ca, authz.ModeEnforce)
	c := client(t, ca)
	tok := "Bearer " + mintToken(t, "ucp", "applications-rp")

	require.Equal(t, http.StatusOK, send(t, c, "https://"+srv.url, tok))
	<-srv.seen

	srv.keys.unavailable.Store(true)
	srv.clock.Advance(token.DefaultMaxKeyAge + time.Minute)
	require.Equal(t, http.StatusServiceUnavailable, send(t, c, "https://"+srv.url, tok))
	requireLogged(t, srv.logs, authz.ModeEnforce, v1.CodeAuthorizationUnavailable, "signing keys")
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

// TestServe_ServingCertificateExpiryOnOpenConnection checks that the receiver stops vouching
// for requests once its own serving certificate expires, even on a connection that was
// established while it was valid, and that a renewed certificate keeps the connection usable.
func TestServe_ServingCertificateExpiryOnOpenConnection(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeEnforce, authz.ModeDryRun} {
		for _, kind := range []string{"local server", "renewed server"} {
			t.Run(string(mode)+"/"+kind, func(t *testing.T) {
				t.Parallel()
				now := time.Now().UTC().Truncate(time.Second)
				expires := now.Add(10 * time.Minute)
				ca := newCA(t)
				leaf := ca.issue(t, certOptions{uris: spiffeURIs("applications-rp"), notAfter: expires})
				var elapsed atomic.Int64
				srv := startRecordingServer(t, ca, mode, serverSetup{
					leaf:    &leaf,
					certNow: func() time.Time { return now.Add(time.Duration(elapsed.Load())) },
				})
				c := client(t, ca)
				transport := c.Transport.(*http.Transport)
				transport.MaxConnsPerHost = 1
				tok := "Bearer " + mintToken(t, "ucp", "applications-rp")

				send := func() (int, bool, bool) {
					t.Helper()
					var reused bool
					trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
					req, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), trace), http.MethodGet, "https://"+srv.url, nil)
					require.NoError(t, err)
					req.Header.Set("Authorization", tok)
					resp, err := c.Do(req)
					require.NoError(t, err)
					defer resp.Body.Close()
					_, err = io.Copy(io.Discard, resp.Body)
					require.NoError(t, err)
					return resp.StatusCode, reused, resp.Close
				}

				status, reused, _ := send()
				require.Equal(t, http.StatusOK, status)
				require.False(t, reused)
				require.True(t, (<-srv.seen).hasIdentity)

				// X.509 validity is inclusive of NotAfter.
				elapsed.Store(int64(expires.Sub(now)))
				status, reused, _ = send()
				require.True(t, reused)
				require.Equal(t, http.StatusOK, status)
				require.True(t, (<-srv.seen).hasIdentity)

				if kind == "renewed server" {
					renewed := ca.issue(t, certOptions{uris: spiffeURIs("applications-rp"), notAfter: now.Add(2 * time.Hour)})
					writeFiles(t, srv.dir, renewed, ca.pem)
					requireServing(t, ca, srv.url, renewed)
				}
				elapsed.Add(1)
				status, reused, closed := send()
				require.True(t, reused, "expiry must be checked on the existing connection")

				if kind == "renewed server" {
					require.Equal(t, http.StatusOK, status)
					require.True(t, (<-srv.seen).hasIdentity)
					for _, entry := range srv.logs.All() {
						m := entry.ContextMap()
						require.Nil(t, m[authz.LogFieldDenied], "unexpected denial: %v", entry)
						require.Nil(t, m[authz.LogFieldWouldDeny], "unexpected denial: %v", entry)
					}
					return
				}
				requireLogged(t, srv.logs, mode, authz.CodeCertificateExpired, "own certificate")
				if mode == authz.ModeEnforce {
					require.Equal(t, http.StatusUnauthorized, status)
					require.True(t, closed)
					require.Empty(t, srv.seen, "the handler must not be called")
					return
				}
				require.Equal(t, http.StatusOK, status)
				seen := <-srv.seen
				require.False(t, seen.hasIdentity, "an expired receiver must not attach an identity")
				require.Empty(t, seen.authorization, "the token must not reach the handler")
			})
		}
	}
}

// requireServing waits until new connections to addr are served with want.
func requireServing(t *testing.T, ca testCA, addr string, want testCert) {
	t.Helper()
	wantLeaf := want.tlsCertificate(t).Leaf
	require.Eventually(t, func() bool {
		conn, err := tls.Dial("tcp", addr, trustClient(t, ca.pem, "applications-rp"))
		if err != nil {
			return false
		}
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0].SerialNumber.Cmp(wantLeaf.SerialNumber) == 0
	}, 5*time.Second, 10*time.Millisecond)
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

func TestServe_NilHandlerUsesDefaultServeMux(t *testing.T) {
	t.Parallel()
	// Repeated runs share DefaultServeMux, so each invocation needs its own route.
	path := "/mtls-test/default-serve-mux/" + rand.Text()
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
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
	// serverDir is laid out like a <receiver>-spiffe volume: tls.crt and tls.key, no ca.crt.
	serverDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(serverDir, CertFile), leaf.certPEM, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(serverDir, KeyFile), leaf.keyPEM, 0o600))
	receiver := Receiver{Name: "applications-rp"}

	for _, tc := range []struct {
		name     string
		options  authz.Options
		receiver Receiver
		want     string
	}{
		{name: "unknown TLS provider", options: authz.Options{Mode: authz.ModeEnforce, TLSProvider: "typo"}, receiver: receiver, want: "invalid TLS provider"},
		{name: "missing directory", options: authz.Options{Mode: authz.ModeEnforce, Namespace: testNamespace}, receiver: receiver, want: "requires certificateDirectory"},
		{name: "missing namespace", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir}, receiver: receiver, want: "requires certificateDirectory and namespace"},
		{name: "missing receiver", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir, Namespace: testNamespace}, want: "receiver name is required"},
		{name: "no certificate files", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: t.TempDir(), Namespace: testNamespace}, receiver: receiver, want: "failed to load server certificate"},
		{name: "server certificate volume without CA directory", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: serverDir, Namespace: testNamespace}, receiver: receiver, want: "failed to read ca.crt"},
		{name: "no CA bundle in CA directory", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir, CADirectory: t.TempDir(), Namespace: testNamespace}, receiver: receiver, want: "failed to read ca.crt"},
		{name: "not in a cluster", options: authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: certDir, Namespace: testNamespace}, receiver: receiver, want: "in-cluster config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			// Fail the in-cluster config deterministically, so the test does not depend on
			// where it runs; inside a pod rest.InClusterConfig succeeds.
			config := serveConfig{
				pollInterval:    testPollInterval,
				inClusterConfig: func() (*rest.Config, error) { return nil, rest.ErrNotInCluster },
			}
			err = serve(t.Context(), &http.Server{}, ln, tc.options, tc.receiver, config)
			require.ErrorContains(t, err, tc.want)

			_, err = ln.Accept()
			require.True(t, errors.Is(err, net.ErrClosed), "listener should be closed: %v", err)
		})
	}

	t.Run("CA bundle from CA directory", func(t *testing.T) {
		t.Parallel()
		options := authz.Options{Mode: authz.ModeDryRun, CertificateDirectory: serverDir, CADirectory: certDir, Namespace: testNamespace}
		receiver := receiver
		receiver.KeySource = &staticKeys{}
		verifier, source, err := newServerAuth(options, receiver, serveConfig{})
		require.NoError(t, err)
		require.NotNil(t, verifier)
		m, err := source.load()
		require.NoError(t, err)
		require.Equal(t, leaf.tlsCertificate(t).Certificate, m.cert.Certificate)
	})
}

// writeToken writes tok as the projected token for receiver under dir.
func writeToken(t *testing.T, dir, receiver, tok string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, receiver), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, receiver, TokenFile), []byte(tok+"\n"), 0o600))
}

// TestServe_StaleSigningKeysFailClosed checks that once the signing keys are MaxKeyAge old,
// every request is rejected with AuthorizationUnavailable in every enforcing mode, whatever
// credentials it carries, and never reaches the handler.
// TestServe_ExpiredServingLeafAndStaleKeys checks that stale signing keys take precedence over
// an expired serving certificate on a reused connection: the request fails with 503 in every
// mode and never reaches the handler.
func TestServe_ExpiredServingLeafAndStaleKeys(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeEnforce, authz.ModeDryRun} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			now := time.Now().UTC().Truncate(time.Second)
			ca := newCA(t)
			leaf := ca.issue(t, certOptions{uris: spiffeURIs("applications-rp"), notAfter: now.Add(time.Hour)})
			var expired atomic.Bool
			srv := startRecordingServer(t, ca, mode, serverSetup{
				leaf: &leaf,
				certNow: func() time.Time {
					if expired.Load() {
						return now.Add(2 * time.Hour)
					}
					return now
				},
			})
			c := client(t, ca)
			tok := "Bearer " + mintToken(t, "ucp", "applications-rp")
			require.Equal(t, http.StatusOK, send(t, c, "https://"+srv.url, tok))
			<-srv.seen

			srv.keys.unavailable.Store(true)
			srv.clock.Advance(token.DefaultMaxKeyAge)
			expired.Store(true)

			var reused bool
			trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) { reused = info.Reused }}
			req, err := http.NewRequestWithContext(httptrace.WithClientTrace(t.Context(), trace), http.MethodGet, "https://"+srv.url, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", tok)
			resp, err := c.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.True(t, reused, "the request must reuse the connection opened before the certificate expired")
			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
			requireLogged(t, srv.logs, authz.ModeEnforce, v1.CodeAuthorizationUnavailable, "signing keys")
			require.Empty(t, srv.seen, "requests must not reach the handler while the signing keys are stale")
		})
	}
}

func TestServe_StaleSigningKeysFailClosed(t *testing.T) {
	t.Parallel()
	credentials := map[string]string{
		"valid token": "Bearer " + mintToken(t, "ucp", "applications-rp"),
		"no token":    "",
		"basic":       "Basic dXNlcjpwYXNz",
	}
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			ca := newCA(t)
			srv := startServer(t, ca, mode)
			c := client(t, ca)

			// Load the keys, then stop every later refresh from succeeding.
			require.Equal(t, http.StatusOK, send(t, c, "https://"+srv.url, credentials["valid token"]))
			<-srv.seen
			srv.keys.unavailable.Store(true)
			srv.clock.Advance(token.DefaultMaxKeyAge)

			for name, authorization := range credentials {
				t.Run(name, func(t *testing.T) {
					require.Equal(t, http.StatusServiceUnavailable, send(t, c, "https://"+srv.url, authorization))
					requireLogged(t, srv.logs, authz.ModeEnforce, v1.CodeAuthorizationUnavailable, "signing keys")
				})
			}
			require.Empty(t, srv.seen, "requests must not reach the handler while the signing keys are stale")
		})
	}
}

// TestServe_RequestTrailers sends a chunked request with a trailer through the client and
// upgrade transports and checks that the trailer, which the caller sets after it writes the
// body, reaches a handler that reads the body.
func TestServe_RequestTrailers(t *testing.T) {
	t.Parallel()
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		for _, transport := range []string{"client", "upgrade"} {
			t.Run(string(mode)+"/"+transport, func(t *testing.T) {
				t.Parallel()
				ca := newCA(t)
				type received struct {
					body     string
					trailer  string
					identity string
				}
				got := make(chan received, 1)
				srv := startServerWithHandler(t, ca, mode, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					id, _ := authz.IdentityFromContext(r.Context())
					got <- received{body: string(body), trailer: r.Trailer.Get("X-Checksum"), identity: id.String()}
					w.WriteHeader(http.StatusOK)
				}))

				options := callerOptions(t, ca.pem, mode, "ucp")
				var rt http.RoundTripper
				url := "https://applications-rp.radius-system.svc:5443/"
				if transport == "client" {
					secure, err := newClientTransport(options, "applications-rp")
					require.NoError(t, err)
					secure.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
						return (&net.Dialer{}).DialContext(ctx, network, srv.url)
					}
					t.Cleanup(secure.CloseIdleConnections)
					rt = secure
				} else {
					var err error
					rt, err = NewUpgradeTransport(options, http.DefaultTransport, []string{"applications-rp"})
					require.NoError(t, err)
					dialTo(t, rt, "applications-rp.radius-system.svc", srv.url)
					url = "http://applications-rp.radius-system.svc:5443/"
				}

				pr, pw := io.Pipe()
				req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, pr)
				require.NoError(t, err)
				req.ContentLength = -1
				req.Trailer = http.Header{"X-Checksum": nil}
				go func() {
					_, _ = pw.Write([]byte("streamed body"))
					// The caller sets the trailer value only after it has written the body.
					req.Trailer.Set("X-Checksum", "abc123")
					_ = pw.Close()
				}()

				resp, err := (&http.Client{Transport: rt, Timeout: 10 * time.Second}).Do(req)
				require.NoError(t, err)
				_ = resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Equal(t, received{body: "streamed body", trailer: "abc123", identity: "ucp.radius-system.svc"}, <-got)
				require.Empty(t, req.Header.Get("Authorization"), "the transport must not modify the caller's request")
			})
		}
	}
}
