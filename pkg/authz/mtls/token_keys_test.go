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
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/golang-jwt/jwt/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"k8s.io/client-go/rest"
)

func TestExternalSigningKeyAlgorithms(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	encode := base64.RawURLEncoding.EncodeToString
	for _, tt := range []struct {
		name   string
		jwk    map[string]any
		key    any
		method jwt.SigningMethod
	}{
		{"RSA", map[string]any{"kid": "rsa", "kty": "RSA", "alg": "RS256", "n": encode(rsaKey.N.Bytes()), "e": encode(big.NewInt(int64(rsaKey.E)).Bytes())}, rsaKey, jwt.SigningMethodRS256},
		{"P384", map[string]any{"kid": "p384", "kty": "EC", "crv": "P-384", "x": encode(p384.X.FillBytes(make([]byte, 48))), "y": encode(p384.Y.FillBytes(make([]byte, 48)))}, p384, jwt.SigningMethodES384},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.jwk)
			require.NoError(t, err)
			id, key, err := parseTokenKey(raw)
			require.NoError(t, err)
			v, _, now := tokenFixture(t)
			v.keys.set = &tokenKeySet{issuer: "https://cluster.example", keys: map[string]tokenSigningKey{id: key}}
			_, code := v.verify(t.Context(), signedToken(t, tt.key, validTokenClaims(now.Now()), id, tt.method))
			require.Empty(t, code)
		})
	}
}

func TestExternalRejectsMalformedSigningKeys(t *testing.T) {
	_, key, _ := tokenFixture(t)
	for _, tt := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"missing kid", func(k map[string]any) { delete(k, "kid") }},
		{"key encryption", func(k map[string]any) { k["use"] = "enc" }},
		{"sign operation", func(k map[string]any) { k["key_ops"] = []string{"sign"} }},
		{"wrong algorithm", func(k map[string]any) { k["alg"] = "HS256" }},
		{"unsupported curve", func(k map[string]any) { k["crv"] = "P-521" }},
		{"off curve", func(k map[string]any) { k["x"] = base64.RawURLEncoding.EncodeToString(make([]byte, 32)) }},
		{"malformed coordinate", func(k map[string]any) { k["x"] = "!invalid" }},
		{"private key", func(k map[string]any) { k["d"] = "private" }},
		{"symmetric key", func(k map[string]any) { k["kty"], k["k"] = "oct", "secret" }},
		{"weak RSA", func(k map[string]any) { k["kty"], k["alg"], k["n"], k["e"] = "RSA", "RS256", "AQ", "AQAB" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			jwk := publicJWK(t, key)
			tt.change(jwk)
			raw, err := json.Marshal(jwk)
			require.NoError(t, err)
			_, _, err = parseTokenKey(raw)
			require.Error(t, err)
		})
	}
}

func TestExternalDiscoveryFailuresAndCancellation(t *testing.T) {
	for _, failure := range []string{"discovery unavailable", "invalid discovery", "bad issuer", "keys unavailable", "invalid keys", "redirect"} {
		t.Run(failure, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/.well-known/openid-configuration" {
					switch failure {
					case "discovery unavailable":
						w.WriteHeader(http.StatusServiceUnavailable)
					case "invalid discovery":
						_, _ = w.Write([]byte("{"))
					case "bad issuer":
						_, _ = w.Write([]byte(`{"issuer":"http://not-trusted","jwks_uri":"https://ignored.example"}`))
					case "redirect":
						http.Redirect(w, r, "https://never-fetch.example", http.StatusFound)
					default:
						_, _ = w.Write([]byte(`{"issuer":"https://cluster.example","jwks_uri":"https://ignored.example"}`))
					}
					return
				}
				if failure == "keys unavailable" {
					w.WriteHeader(http.StatusForbidden)
				} else {
					_, _ = w.Write([]byte(`{"keys":[`))
				}
			}))
			defer server.Close()
			fetch, err := clusterTokenKeyFetcher(&rest.Config{Host: server.URL, Transport: server.Client().Transport})
			require.NoError(t, err)
			v, _, _ := tokenFixture(t)
			v.keys.set = nil
			v.keys.fetch = fetch
			require.Error(t, v.keys.update(t.Context(), false))
			_, code := v.verify(t.Context(), "")
			require.Equal(t, v1.CodeAuthorizationUnavailable, code)
		})
	}
	v, _, _ := tokenFixture(t)
	last := v.keys.fetched
	v.keys.fetch = func(ctx context.Context) (*tokenKeySet, error) { <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, v.keys.update(ctx, false), context.Canceled)
	require.Equal(t, last, v.keys.fetched)
}

func TestExternalDryRunLogsWithoutTrustingIdentity(t *testing.T) {
	v, _, _ := tokenFixture(t)
	core, logs := observer.New(zap.InfoLevel)
	ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
	ctx = WithIdentity(ctx, Identity{Component: "ucp", Namespace: "radius-system"})
	request := httptest.NewRequest(http.MethodGet, "/resource", nil).WithContext(ctx)
	request.Header.Set("X-Forwarded-Client-Cert", "forged identity")
	var called bool
	handler := v.authorizeToken(authz.ModeDryRun, nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, found := IdentityFromContext(r.Context())
		require.False(t, found)
		w.WriteHeader(http.StatusNoContent)
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.True(t, called)
	require.Equal(t, 204, recorder.Code)
	entry := logs.All()[0].ContextMap()
	require.Equal(t, true, entry[authz.LogFieldWouldDeny])
	require.Equal(t, v1.CodeInvalidAuthenticationInfo, entry[authz.LogFieldCode])
}

func TestExternalTokenExpiryOnReusedConnection(t *testing.T) {
	v, key, now := tokenFixture(t)
	claims := validTokenClaims(now.Now())
	raw := signedToken(t, key, claims, "key-1", jwt.SigningMethodES256)
	var called atomic.Int32
	server := httptest.NewServer(v.authorizeToken(authz.ModeEnforce, nil, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})))
	defer server.Close()
	for _, elapsed := range []time.Duration{0, 10 * time.Minute} {
		now.SetTime(claims.IssuedAt.Add(elapsed))
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+raw)
		resp, err := server.Client().Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		if elapsed == 0 {
			require.Equal(t, 204, resp.StatusCode)
		} else {
			require.Equal(t, 401, resp.StatusCode)
		}
	}
	require.EqualValues(t, 1, called.Load())
}

func TestExternalServerInitializationAndShutdown(t *testing.T) {
	t.Setenv("RADIUS_INTERNAL_AUTH_POD_UID", "22222222-2222-4222-8222-222222222222")
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "discovery succeeds", true: "discovery unavailable"}[unavailable], func(t *testing.T) {
			_, key, _ := tokenFixture(t)
			var calls atomic.Int32
			api := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if unavailable {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				if r.URL.Path == "/.well-known/openid-configuration" {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"issuer": "https://cluster.example", "jwks_uri": "https://ignored.example/keys"}))
				} else {
					require.Equal(t, "/openid/v1/jwks", r.URL.Path)
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"keys": []any{publicJWK(t, key)}}))
				}
			}))
			defer api.Close()
			config := &rest.Config{Host: api.URL, Transport: api.Client().Transport}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				id, ok := IdentityFromContext(r.Context())
				require.True(t, ok)
				require.Equal(t, "ucp.radius-system.svc", id.String())
				w.WriteHeader(http.StatusNoContent)
			})}
			defer server.Close()
			done := make(chan error, 1)
			go func() {
				done <- serveExternalWithConfig(t.Context(), server, listener,
					authz.Options{Mode: authz.ModeEnforce, MTLSProvider: "external", Component: "applications-rp", Namespace: "radius-system"},
					[]string{"ucp"}, config)
			}()
			request, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+"/resource", nil)
			require.NoError(t, err)
			request.Header.Set("Authorization", "Bearer "+signedToken(t, key, validTokenClaims(time.Now()), "key-1", jwt.SigningMethodES256))
			client := &http.Client{Timeout: time.Second}
			response, err := client.Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			if unavailable {
				require.Equal(t, 503, response.StatusCode)
				require.EqualValues(t, 1, calls.Load())
			} else {
				require.Equal(t, 204, response.StatusCode)
				require.EqualValues(t, 2, calls.Load())
			}
			probe := request.Clone(t.Context())
			probe.URL.Path = InternalIdentityPath
			probe.URL.RawQuery = "challenge=11111111-1111-4111-8111-111111111111"
			response, err = client.Do(probe)
			require.NoError(t, err)
			if unavailable {
				require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
			} else {
				require.Equal(t, http.StatusOK, response.StatusCode)
				var receipt InternalIdentityReceipt
				require.NoError(t, json.NewDecoder(response.Body).Decode(&receipt))
				require.Equal(t, "22222222-2222-4222-8222-222222222222", receipt.PodUID)
				require.Equal(t, "applications-rp", receipt.Component)
				require.Equal(t, "system:serviceaccount:radius-system:ucp", receipt.Caller)
			}
			require.NoError(t, response.Body.Close())
			require.NoError(t, server.Close())
			select {
			case err := <-done:
				require.ErrorIs(t, err, http.ErrServerClosed)
			case <-time.After(time.Second):
				t.Fatal("server/key-refresh shutdown did not finish")
			}
		})
	}
}

func TestExternalServeOffAndMissingClusterConfiguration(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	err = Serve(t.Context(), &http.Server{}, listener, authz.Options{Mode: authz.ModeEnforce, MTLSProvider: "external"}, nil)
	require.ErrorContains(t, err, "in-cluster")
	require.Error(t, listener.Close())
	// Off never reads in-cluster credentials or requires projected token files.
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })}
	listener, err = net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- Serve(t.Context(), server, listener, authz.Options{MTLSProvider: "external"}, nil) }()
	response, err := (&http.Client{Timeout: time.Second}).Get("http://" + listener.Addr().String())
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 204, response.StatusCode)
	require.NoError(t, server.Close())
	require.ErrorIs(t, <-done, http.ErrServerClosed)
}
