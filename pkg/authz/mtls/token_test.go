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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	clocktesting "k8s.io/utils/clock/testing"
)

func tokenFixture(t *testing.T) (*tokenVerifier, *ecdsa.PrivateKey, *clocktesting.FakeClock) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	now := clocktesting.NewFakeClock(time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	keys := &tokenKeys{clock: now, fetch: func(context.Context) (*tokenKeySet, error) {
		return &tokenKeySet{issuer: "https://cluster.example", keys: map[string]tokenSigningKey{
			"key-1": {algorithm: "ES256", public: &key.PublicKey},
		}}, nil
	}}
	require.NoError(t, keys.update(t.Context(), false))
	return &tokenVerifier{keys: keys, receiver: "applications-rp", namespace: "radius-system"}, key, now
}

func validTokenClaims(now time.Time) *componentTokenClaims {
	c := &componentTokenClaims{RegisteredClaims: jwt.RegisteredClaims{
		Issuer: "https://cluster.example", Subject: "system:serviceaccount:radius-system:ucp",
		Audience: jwt.ClaimStrings{"applications-rp.radius-system.svc"},
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
	}}
	c.Kubernetes.Namespace = "radius-system"
	c.Kubernetes.ServiceAccount.Name, c.Kubernetes.ServiceAccount.UID = "ucp", "uid"
	return c
}

func signedToken(t *testing.T, key any, claims *componentTokenClaims, kid string, method jwt.SigningMethod) string {
	t.Helper()
	token := jwt.NewWithClaims(method, claims)
	token.Header["kid"] = kid
	raw, err := token.SignedString(key)
	require.NoError(t, err)
	return raw
}

func TestExternalTokenClaims(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*componentTokenClaims)
		valid  bool
	}{
		{"valid", func(*componentTokenClaims) {}, true},
		{"issuer", func(c *componentTokenClaims) { c.Issuer = "https://attacker.example" }, false},
		{"replay", func(c *componentTokenClaims) { c.Audience = jwt.ClaimStrings{"ucp.radius-system.svc"} }, false},
		{"multiple audiences", func(c *componentTokenClaims) { c.Audience = append(c.Audience, "ucp.radius-system.svc") }, false},
		{"missing audience", func(c *componentTokenClaims) { c.Audience = nil }, false},
		{"namespace", func(c *componentTokenClaims) { c.Kubernetes.Namespace = "tenant" }, false},
		{"unknown service account", func(c *componentTokenClaims) { c.Kubernetes.ServiceAccount.Name = "tenant" }, false},
		{"subject", func(c *componentTokenClaims) { c.Subject = "system:serviceaccount:radius-system:controller" }, false},
		{"missing uid", func(c *componentTokenClaims) { c.Kubernetes.ServiceAccount.UID = "" }, false},
		{"expiry exact", func(c *componentTokenClaims) { c.ExpiresAt = c.IssuedAt }, false},
		{"missing expiry", func(c *componentTokenClaims) { c.ExpiresAt = nil }, false},
		{"missing issued at", func(c *componentTokenClaims) { c.IssuedAt = nil }, false},
		{"issued in future", func(c *componentTokenClaims) { c.IssuedAt = jwt.NewNumericDate(c.IssuedAt.Add(time.Second)) }, false},
		{"missing not before", func(c *componentTokenClaims) { c.NotBefore = nil }, false},
		{"not before future", func(c *componentTokenClaims) { c.NotBefore = jwt.NewNumericDate(c.NotBefore.Add(time.Second)) }, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v, key, now := tokenFixture(t)
			c := validTokenClaims(now.Now())
			tt.mutate(c)
			id, code := v.verify(t.Context(), signedToken(t, key, c, "key-1", jwt.SigningMethodES256))
			if tt.valid {
				require.Empty(t, code)
				require.Equal(t, "ucp.radius-system.svc", id.String())
			} else {
				require.Equal(t, v1.CodeInvalidAuthenticationInfo, code)
				require.Empty(t, id.Component)
			}
		})
	}
}

func TestExternalTokenSignatures(t *testing.T) {
	v, key, now := tokenFixture(t)
	wrongKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	for name, token := range map[string]string{
		"wrong signature": signedToken(t, wrongKey, validTokenClaims(now.Now()), "key-1", jwt.SigningMethodES256),
		"unknown kid":     signedToken(t, key, validTokenClaims(now.Now()), "unknown", jwt.SigningMethodES256),
		"missing kid":     signedToken(t, key, validTokenClaims(now.Now()), "", jwt.SigningMethodES256),
		"symmetric":       signedToken(t, []byte("not-a-cluster-key"), validTokenClaims(now.Now()), "key-1", jwt.SigningMethodHS256),
		"unsigned":        signedToken(t, jwt.UnsafeAllowNoneSignatureType, validTokenClaims(now.Now()), "key-1", jwt.SigningMethodNone),
		"malformed":       "not.a.token",
		"missing":         "",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := v.verify(t.Context(), token)
			require.Equal(t, v1.CodeInvalidAuthenticationInfo, code)
		})
	}
}

func TestExternalTokenRequiresIntegerTimes(t *testing.T) {
	for _, field := range []string{"exp", "iat", "nbf"} {
		for _, kind := range []string{"string", "fractional", "null"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				t.Parallel()
				v, key, now := tokenFixture(t)
				raw, err := json.Marshal(validTokenClaims(now.Now()))
				require.NoError(t, err)
				var claims jwt.MapClaims
				require.NoError(t, json.Unmarshal(raw, &claims))
				switch kind {
				case "string":
					claims[field] = fmt.Sprintf("%.0f", claims[field])
				case "fractional":
					claims[field] = claims[field].(float64) + 0.5
				case "null":
					claims[field] = nil
				}
				token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
				token.Header["kid"] = "key-1"
				signed, err := token.SignedString(key)
				require.NoError(t, err)
				_, code := v.verify(t.Context(), signed)
				require.Equal(t, v1.CodeInvalidAuthenticationInfo, code)
			})
		}
	}
}

func TestExternalKeyFreshnessAndRecovery(t *testing.T) {
	v, key, now := tokenFixture(t)
	v.keys.fetch = func(context.Context) (*tokenKeySet, error) { return nil, errors.New("unavailable") }
	for _, age := range []time.Duration{5 * time.Minute, 15*time.Minute - time.Nanosecond, 15 * time.Minute, 16 * time.Minute} {
		t.Run(age.String(), func(t *testing.T) {
			now.SetTime(v.keys.fetched.Add(age))
			require.Error(t, v.keys.update(t.Context(), false))
			// Fresh token isolates key freshness from token expiry.
			raw := signedToken(t, key, validTokenClaims(now.Now()), "key-1", jwt.SigningMethodES256)
			_, code := v.verify(t.Context(), raw)
			if age < 15*time.Minute {
				require.Empty(t, code)
			} else {
				require.Equal(t, v1.CodeAuthorizationUnavailable, code)
			}
		})
	}
	set := v.keys.set
	v.keys.fetch = func(context.Context) (*tokenKeySet, error) { return set, nil }
	require.NoError(t, v.keys.update(t.Context(), false))
	_, fresh := v.keys.snapshot()
	require.True(t, fresh)
}

func TestExternalKeyRefreshScheduleCancellation(t *testing.T) {
	v, _, now := tokenFixture(t)
	var count atomic.Int32
	updated := make(chan struct{}, 1)
	set := v.keys.set
	v.keys.fetch = func(context.Context) (*tokenKeySet, error) {
		count.Add(1)
		updated <- struct{}{}
		return set, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); v.keys.run(ctx) }()
	require.Eventually(t, now.HasWaiters, time.Second, time.Millisecond)
	now.Step(5*time.Minute - time.Nanosecond)
	require.Zero(t, count.Load())
	now.Step(time.Nanosecond)
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Fatal("no refresh at exactly five minutes")
	}
	require.EqualValues(t, 1, count.Load())
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("key refresh did not stop")
	}
}

func TestExternalKeyRotationUnknownThrottle(t *testing.T) {
	v, oldKey, now := tokenFixture(t)
	newKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	var fetches atomic.Int32
	v.keys.fetch = func(context.Context) (*tokenKeySet, error) {
		fetches.Add(1)
		return &tokenKeySet{issuer: "https://cluster.example", keys: map[string]tokenSigningKey{
			"key-2": {algorithm: "ES256", public: &newKey.PublicKey},
		}}, nil
	}
	raw := signedToken(t, newKey, validTokenClaims(now.Now()), "key-2", jwt.SigningMethodES256)
	_, code := v.verify(t.Context(), raw)
	require.Equal(t, v1.CodeInvalidAuthenticationInfo, code)
	require.Zero(t, fetches.Load())
	now.Step(30 * time.Second)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, code := v.verify(t.Context(), raw)
			require.Empty(t, code)
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, fetches.Load())
	_, code = v.verify(t.Context(), signedToken(t, oldKey, validTokenClaims(now.Now()), "key-1", jwt.SigningMethodES256))
	require.Equal(t, v1.CodeInvalidAuthenticationInfo, code)
}

func TestExternalHTTPRequestBoundary(t *testing.T) {
	for _, mode := range []authz.Mode{authz.ModeDryRun, authz.ModeEnforce} {
		for _, kind := range []string{"valid", "forged", "replay", "wrong caller", "stale", "duplicate"} {
			t.Run(string(mode)+"/"+kind, func(t *testing.T) {
				v, key, now := tokenFixture(t)
				claims := validTokenClaims(now.Now())
				if kind == "replay" {
					claims.Audience = jwt.ClaimStrings{"ucp.radius-system.svc"}
				}
				if kind == "wrong caller" {
					claims.Subject = "system:serviceaccount:radius-system:controller"
					claims.Kubernetes.ServiceAccount.Name = "controller"
				}
				raw := signedToken(t, key, claims, "key-1", jwt.SigningMethodES256)
				if kind == "forged" {
					raw = "forged"
				}
				if kind == "stale" {
					now.Step(15 * time.Minute)
				}
				var called atomic.Bool
				server := httptest.NewServer(v.authorizeToken(mode, []string{"ucp"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					called.Store(true)
					id, ok := IdentityFromContext(r.Context())
					require.Equal(t, kind == "valid" || kind == "wrong caller", ok)
					if ok {
						require.Equal(t, claims.Kubernetes.ServiceAccount.Name, id.Component)
					}
					for _, h := range []string{"Authorization", "X-Remote-User", "X-Remote-Group", "X-Remote-Extra-Scope", "X-Forwarded-Client-Cert"} {
						require.Empty(t, r.Header.Get(h))
					}
					require.Equal(t, "correlation", r.Header.Get("X-Ms-Correlation-Request-Id"))
					w.WriteHeader(http.StatusNoContent)
				})))
				defer server.Close()
				req, err := http.NewRequest(http.MethodGet, server.URL+"/resource", nil)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+raw)
				if kind == "duplicate" {
					req.Header.Add("Authorization", "Bearer "+raw)
				}
				req.Header.Set("X-Remote-User", "admin")
				req.Header.Set("X-Remote-Group", "system:masters")
				req.Header.Set("X-Remote-Extra-Scope", "all")
				req.Header.Set("X-Forwarded-Client-Cert", "forged")
				req.Header.Set("X-Ms-Correlation-Request-Id", "correlation")
				resp, err := server.Client().Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				expected := http.StatusNoContent
				if kind == "stale" {
					expected = http.StatusServiceUnavailable
				} else if mode == authz.ModeEnforce && kind != "valid" {
					expected = http.StatusUnauthorized
					if kind == "wrong caller" {
						expected = http.StatusForbidden
					}
				}
				require.Equal(t, expected, resp.StatusCode)
				require.Equal(t, expected == http.StatusNoContent, called.Load())
				if expected != http.StatusNoContent {
					var body v1.ErrorResponse
					require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
					require.NotEmpty(t, body.Error.Code)
				}
			})
		}
	}
}

func TestExternalProjectedTokenTransport(t *testing.T) {
	v, key, now := tokenFixture(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "applications-rp")
	options := authz.Options{Mode: authz.ModeEnforce, MTLSProvider: "external", Namespace: "radius-system", TokenDirectory: dir}
	transport, err := newTokenTransport(options, []string{"applications-rp"})
	require.NoError(t, err)
	var got atomic.Int32
	server := httptest.NewServer(v.authorizeToken(authz.ModeEnforce, []string{"ucp"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Add(1)
		require.Equal(t, "metadata", r.Header.Get("X-Ms-Client-Request-Id"))
		w.WriteHeader(http.StatusNoContent)
	})))
	defer server.Close()
	base := transport.(*tokenTransport).base.(*http.Transport)
	defer base.CloseIdleConnections()
	base.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}

	req, err := http.NewRequest(http.MethodGet, "http://applications-rp.radius-system.svc:5443/resource", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "original")
	req.Header.Set("X-Ms-Client-Request-Id", "metadata")
	_, err = transport.RoundTrip(req)
	require.ErrorContains(t, err, "cannot read")
	for _, receiver := range []string{"applications-rp", "ucp", "applications-rp"} {
		claims := validTokenClaims(now.Now())
		claims.Audience = jwt.ClaimStrings{receiver + ".radius-system.svc"}
		token := signedToken(t, key, claims, "key-1", jwt.SigningMethodES256)
		// Atomic replacement models kubelet projection, rather than a cached open file.
		require.NoError(t, os.WriteFile(path+".new", []byte(token), 0600))
		require.NoError(t, os.Rename(path+".new", path))
		resp, err := transport.RoundTrip(req)
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		if receiver == "ucp" {
			require.Equal(t, 401, resp.StatusCode)
		} else {
			require.Equal(t, 204, resp.StatusCode)
		}
		require.Equal(t, "original", req.Header.Get("Authorization"))
	}
	require.EqualValues(t, 2, got.Load())
	req.URL.Host = "attacker.example"
	_, err = transport.RoundTrip(req)
	require.ErrorContains(t, err, "destination")
}

func TestExternalClientOffRequiresNoCredentials(t *testing.T) {
	transport, err := NewClientTransport(authz.Options{MTLSProvider: "external"})
	require.NoError(t, err)
	require.Same(t, http.DefaultTransport, transport)
}

func TestExternalUpgradeTransportSeparatesReceiverCredentials(t *testing.T) {
	dir := t.TempDir()
	options := authz.Options{Mode: authz.ModeEnforce, MTLSProvider: "external", Namespace: "radius-system", TokenDirectory: dir}
	plain := &recordingTransport{}
	transport, err := NewUpgradeTransport(options, plain, []string{"applications-rp", "dynamic-rp", "bicep-de"})
	require.NoError(t, err)
	for receiver, port := range map[string]string{"applications-rp": "5443", "dynamic-rp": "8082", "bicep-de": "6443"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, receiver), []byte(receiver+"-token"), 0600))
		transport.(*upgradeTransport).secure[receiver].(*tokenTransport).base = &recordingTransport{}
		request, err := http.NewRequest(http.MethodGet, "http://"+receiver+".radius-system.svc:"+port+"/resource", nil)
		require.NoError(t, err)
		request.Header.Set("X-Ms-Client-Request-Id", "metadata")
		response, err := transport.RoundTrip(request)
		require.NoError(t, err)
		require.Equal(t, "Bearer "+receiver+"-token", response.Request.Header.Get("Authorization"))
		require.Equal(t, "metadata", response.Request.Header.Get("X-Ms-Client-Request-Id"))
		require.Empty(t, request.Header.Get("Authorization"))
		require.Equal(t, "http", response.Request.URL.Scheme)
		request.URL.Scheme = "https"
		_, err = transport.RoundTrip(request)
		require.ErrorContains(t, err, "destination")
	}
	request, err := http.NewRequest(http.MethodGet, "https://unrelated.example/resource", nil)
	require.NoError(t, err)
	response, err := transport.RoundTrip(request)
	require.NoError(t, err)
	require.Empty(t, response.Request.Header.Get("Authorization"))
	require.Len(t, plain.urls, 1)
}

func publicJWK(t *testing.T, key *ecdsa.PrivateKey) map[string]any {
	t.Helper()
	return map[string]any{"kid": "key-1", "kty": "EC", "crv": "P-256", "alg": "ES256", "use": "sig",
		"x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		"y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}
}

func TestExternalDiscoveryValidatesWholeSet(t *testing.T) {
	v, key, now := tokenFixture(t)
	good := publicJWK(t, key)
	var keys any = []any{good}
	issuer := "https://cluster.example"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": "https://never-fetch.example/keys"}))
		case "/openid/v1/jwks":
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"keys": keys}))
		default:
			t.Errorf("unexpected discovery destination: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	config := &rest.Config{Host: server.URL, Transport: server.Client().Transport}
	fetch, err := clusterTokenKeyFetcher(config)
	require.NoError(t, err)
	v.keys.fetch = fetch
	require.NoError(t, v.keys.update(t.Context(), false))
	lastGood := v.keys.fetched
	for _, invalid := range []any{
		[]any{}, []any{good, good}, []any{good, map[string]any{"kid": "bad", "kty": "oct"}},
		[]any{map[string]any{"kid": "bad", "kty": "EC", "crv": "P-256", "x": "AA", "y": "AA"}},
	} {
		now.Step(time.Minute)
		keys = invalid
		require.Error(t, v.keys.update(t.Context(), false))
		require.Equal(t, lastGood, v.keys.fetched)
	}
	keys = []any{good}
	issuer = "https://other.example"
	require.ErrorContains(t, v.keys.update(t.Context(), false), "issuer changed")
	require.Equal(t, lastGood, v.keys.fetched)
	config.Insecure = true
	_, err = clusterTokenKeyFetcher(config)
	require.Error(t, err)
}
