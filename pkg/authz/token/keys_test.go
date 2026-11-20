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

package token

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"k8s.io/client-go/rest"
)

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func rsaJWK(kid string, key *rsa.PublicKey) jsonWebKey {
	return jsonWebKey{Kty: "RSA", Kid: kid, Use: "sig", N: b64(key.N.Bytes()), E: b64(big.NewInt(int64(key.E)).Bytes())}
}

func ecJWK(t *testing.T, kid string, key *ecdsa.PublicKey) jsonWebKey {
	t.Helper()
	point, err := key.Bytes()
	require.NoError(t, err)
	size := (key.Curve.Params().BitSize + 7) / 8
	crv := key.Curve.Params().Name
	return jsonWebKey{Kty: "EC", Kid: kid, Crv: crv, X: b64(point[1 : 1+size]), Y: b64(point[1+size:])}
}

func TestPublicKeys(t *testing.T) {
	smallRSA, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	p521, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	require.NoError(t, err)

	encryption := rsaJWK("enc", &rsaKey.PublicKey)
	encryption.Use = "enc"
	badPoint := ecJWK(t, "bad-point", &ecKey.PublicKey)
	badPoint.Y = badPoint.X

	set := jsonWebKeySet{Keys: []jsonWebKey{
		rsaJWK("rsa", &rsaKey.PublicKey),
		ecJWK(t, "p256", &ecKey.PublicKey),
		ecJWK(t, "p384", &p384.PublicKey),
		rsaJWK("small", &smallRSA.PublicKey),
		ecJWK(t, "p521", &p521.PublicKey),
		rsaJWK("", &rsaKey.PublicKey),
		encryption,
		badPoint,
		{Kty: "oct", Kid: "symmetric"},
	}}

	keys, err := set.publicKeys()
	require.NoError(t, err)
	require.Len(t, keys, 3)
	require.True(t, rsaKey.PublicKey.Equal(keys["rsa"]))
	require.True(t, ecKey.PublicKey.Equal(keys["p256"]))
	require.True(t, p384.PublicKey.Equal(keys["p384"]))
}

func TestOIDCKeySource_MissingKeysVersusEmptyKeys(t *testing.T) {
	valid, err := json.Marshal(jsonWebKeySet{Keys: []jsonWebKey{rsaJWK("rsa", &rsaKey.PublicKey)}})
	require.NoError(t, err)
	tests := []struct {
		body      string
		keepsKeys bool
	}{
		{body: `{}`, keepsKeys: true},
		{body: `null`, keepsKeys: true},
		{body: `{"keys":null}`, keepsKeys: true},
		{body: `{"keys":[]}`, keepsKeys: false},
	}
	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			var body atomic.Pointer[string]
			body.Store(new(string(valid)))
			mux := http.NewServeMux()
			var srv *httptest.Server
			mux.HandleFunc(discoveryPath, func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(discoveryDocument{Issuer: testIssuer, JWKSURI: srv.URL + apiServerJWKSPath})
			})
			mux.HandleFunc(apiServerJWKSPath, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", jwkSetMediaType)
				_, _ = w.Write([]byte(*body.Load()))
			})
			srv = httptest.NewTLSServer(mux)
			t.Cleanup(srv.Close)

			v := newTestVerifier(t, &OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}, &fakeClock{now: start})
			require.NoError(t, v.Refresh(t.Context()))
			token := sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start))

			body.Store(&tt.body)
			require.Error(t, v.Refresh(t.Context()))
			_, err := v.Verify(t.Context(), token)
			if tt.keepsKeys {
				require.NoError(t, err, "a key set with no keys member is malformed, so the cached keys stay")
			} else {
				requireCode(t, err, v1.CodeAuthorizationUnavailable)
			}
		})
	}
}

func TestPublicKeys_KeyRestrictions(t *testing.T) {
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	with := func(k jsonWebKey, alg string, ops ...string) jsonWebKey {
		k.Alg, k.KeyOps = alg, ops
		return k
	}
	set := jsonWebKeySet{Keys: []jsonWebKey{
		with(rsaJWK("rs256", &rsaKey.PublicKey), "RS256"),
		with(ecJWK(t, "es256", &ecKey.PublicKey), "ES256"),
		with(ecJWK(t, "es384", &p384.PublicKey), "ES384"),
		with(rsaJWK("verify", &rsaKey.PublicKey), "", "verify"),
		with(rsaJWK("ps256", &rsaKey.PublicKey), "PS256"),
		with(ecJWK(t, "p256-es384", &ecKey.PublicKey), "ES384"),
		with(ecJWK(t, "p384-es256", &p384.PublicKey), "ES256"),
		with(rsaJWK("rsa-es256", &rsaKey.PublicKey), "ES256"),
		with(rsaJWK("sign-only", &rsaKey.PublicKey), "", "sign"),
		with(rsaJWK("no-ops", &rsaKey.PublicKey), "", []string{}...),
		// A restricted key may share an ID with a usable one, in either order.
		with(rsaJWK("shared", &rsaKey.PublicKey), "PS256"),
		with(rsaJWK("shared", &rsaKey.PublicKey), "RS256"),
		with(rsaJWK("shared-ops", &rsaKey.PublicKey), "RS256"),
		with(rsaJWK("shared-ops", &rsaKey.PublicKey), "", "encrypt"),
	}}

	keys, err := set.publicKeys()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"rs256", "es256", "es384", "verify", "shared", "shared-ops"}, slices.Collect(maps.Keys(keys)))
}

func TestPublicKeys_Errors(t *testing.T) {
	keys, err := jsonWebKeySet{Keys: []jsonWebKey{{Kty: "oct", Kid: "secret"}}}.publicKeys()
	require.NoError(t, err, "a set with no usable keys is the cluster's answer, not a read failure")
	require.Empty(t, keys)

	_, err = jsonWebKeySet{Keys: []jsonWebKey{rsaJWK("dup", &rsaKey.PublicKey), rsaJWK("dup", &rsaKey.PublicKey)}}.publicKeys()
	require.ErrorContains(t, err, "more than one key")
}

// RFC 7517 section 4.5 allows keys of different types to share an ID. The result must not
// depend on the order of the keys in the set.
func TestPublicKeys_SharedIDWithUnsupportedKey(t *testing.T) {
	supported := rsaJWK("shared", &rsaKey.PublicKey)
	unsupported := jsonWebKey{Kty: "oct", Kid: "shared", Use: "sig"}
	for name, set := range map[string][]jsonWebKey{
		"supported first":   {supported, unsupported},
		"unsupported first": {unsupported, supported},
	} {
		t.Run(name, func(t *testing.T) {
			keys, err := jsonWebKeySet{Keys: set}.publicKeys()
			require.NoError(t, err)
			require.Len(t, keys, 1)
			require.True(t, rsaKey.PublicKey.Equal(keys["shared"]))
		})
	}
}

// newAPIServer serves OIDC discovery with the given jwks_uri and the key set at /openid/v1/jwks.
func newAPIServer(t *testing.T, jwksURI func(base string) string, jwks jsonWebKeySet) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var jwksHits atomic.Int32
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc(discoveryPath, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(discoveryDocument{Issuer: testIssuer, JWKSURI: jwksURI(srv.URL)})
	})
	mux.HandleFunc(apiServerJWKSPath, func(w http.ResponseWriter, r *http.Request) {
		jwksHits.Add(1)
		// Like the Kubernetes API server, serve the key set only to clients that accept its media type.
		if !strings.Contains(r.Header.Get("Accept"), jwkSetMediaType) {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		w.Header().Set("Content-Type", jwkSetMediaType)
		_ = json.NewEncoder(w).Encode(jwks)
	})
	srv = httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv, &jwksHits
}

func TestOIDCKeySource_Fetch(t *testing.T) {
	jwks := jsonWebKeySet{Keys: []jsonWebKey{rsaJWK("rsa", &rsaKey.PublicKey)}}

	t.Run("jwks_uri on API server", func(t *testing.T) {
		srv, hits := newAPIServer(t, func(base string) string { return base + apiServerJWKSPath }, jwks)
		source := &OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}

		keys, err := source.Fetch(t.Context())
		require.NoError(t, err)
		require.Equal(t, testIssuer, keys.Issuer)
		require.True(t, rsaKey.PublicKey.Equal(keys.Keys["rsa"]))
		require.EqualValues(t, 1, hits.Load())
	})

	t.Run("external jwks_uri reads keys from API server", func(t *testing.T) {
		var externalHits atomic.Int32
		external := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			externalHits.Add(1)
		}))
		t.Cleanup(external.Close)

		srv, hits := newAPIServer(t, func(string) string { return external.URL + "/keys.json" }, jwks)
		source := &OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}

		keys, err := source.Fetch(t.Context())
		require.NoError(t, err)
		require.Len(t, keys.Keys, 1)
		require.EqualValues(t, 1, hits.Load())
		require.Zero(t, externalHits.Load(), "credentials must not be sent to another host")
	})
}

func TestOIDCKeySource_FetchErrors(t *testing.T) {
	jwks := jsonWebKeySet{Keys: []jsonWebKey{rsaJWK("rsa", &rsaKey.PublicKey)}}

	t.Run("http base URL", func(t *testing.T) {
		_, err := (&OIDCKeySource{BaseURL: "http://kubernetes.default.svc"}).Fetch(t.Context())
		require.ErrorContains(t, err, "https")
	})

	t.Run("discovery error status", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.NotFoundHandler())
		t.Cleanup(srv.Close)
		_, err := (&OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}).Fetch(t.Context())
		require.ErrorContains(t, err, "status 404")
	})

	t.Run("redirect is not followed", func(t *testing.T) {
		var otherHits atomic.Int32
		other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			otherHits.Add(1)
		}))
		t.Cleanup(other.Close)
		srv := httptest.NewTLSServer(http.RedirectHandler(other.URL+discoveryPath, http.StatusFound))
		t.Cleanup(srv.Close)

		_, err := (&OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}).Fetch(t.Context())
		require.ErrorContains(t, err, "status 302")
		require.Zero(t, otherHits.Load(), "credentials must not follow a redirect")
	})

	t.Run("no jwks_uri", func(t *testing.T) {
		srv, _ := newAPIServer(t, func(string) string { return "" }, jwks)
		_, err := (&OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}).Fetch(t.Context())
		require.ErrorContains(t, err, "no jwks_uri")
	})

	t.Run("no usable keys", func(t *testing.T) {
		srv, _ := newAPIServer(t, func(base string) string { return base + apiServerJWKSPath }, jsonWebKeySet{Keys: []jsonWebKey{}})
		set, err := (&OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}).Fetch(t.Context())
		require.NoError(t, err, "the Verifier, not the source, decides what an empty set means")
		require.NotEmpty(t, set.Issuer)
		require.Empty(t, set.Keys)
	})

	t.Run("oversized response", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat(" ", maxResponseBytes+1)))
		}))
		t.Cleanup(srv.Close)
		_, err := (&OIDCKeySource{Client: srv.Client(), BaseURL: srv.URL}).Fetch(t.Context())
		require.ErrorContains(t, err, "more than")
	})
}

func TestNewOIDCKeySource(t *testing.T) {
	source, err := NewOIDCKeySource(&rest.Config{Host: "https://kubernetes.default.svc", BearerToken: "token"})
	require.NoError(t, err)
	require.Equal(t, "https://kubernetes.default.svc", source.BaseURL)
	require.NotNil(t, source.Client)
}
