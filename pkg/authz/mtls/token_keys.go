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
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
)

const (
	tokenKeyRefreshInterval = 5 * time.Minute
	tokenKeyMaxAge          = 15 * time.Minute
	unknownKeyRefreshDelay  = 30 * time.Second
	tokenDiscoveryTimeout   = 10 * time.Second
)

type tokenSigningKey struct {
	algorithm string
	public    any
}

type tokenKeySet struct {
	issuer string
	keys   map[string]tokenSigningKey
}

type tokenKeyFetcher func(context.Context) (*tokenKeySet, error)

// tokenKeys retains the last completely validated set, never beyond its freshness bound.
// Refreshes are serialized, and requests cannot change the discovery endpoint or issuer.
type tokenKeys struct {
	clock   clock.WithTicker
	fetch   tokenKeyFetcher
	refresh sync.Mutex
	mu      sync.RWMutex
	set     *tokenKeySet
	fetched time.Time
	attempt time.Time
}

func (k *tokenKeys) update(ctx context.Context, unknown bool) error {
	k.refresh.Lock()
	defer k.refresh.Unlock()
	now := k.clock.Now()
	k.mu.Lock()
	if unknown && !k.attempt.IsZero() && now.Sub(k.attempt) < unknownKeyRefreshDelay {
		k.mu.Unlock()
		return nil
	}
	k.attempt = now
	k.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, tokenDiscoveryTimeout)
	defer cancel()
	set, err := k.fetch(ctx)
	if err != nil {
		return err
	}
	if set == nil || set.issuer == "" || len(set.keys) == 0 {
		return errors.New("cluster signing key discovery returned an empty set")
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	// Changing the issuer is a trust-root change, not a signing-key rotation.
	if k.set != nil && k.set.issuer != set.issuer {
		return errors.New("cluster token issuer changed; restart with the intended cluster configuration")
	}
	k.set, k.fetched = set, k.clock.Now()
	return nil
}

func (k *tokenKeys) snapshot() (*tokenKeySet, bool) {
	k.mu.RLock()
	defer k.mu.RUnlock()
	age := k.clock.Now().Sub(k.fetched)
	return k.set, k.set != nil && age >= 0 && age < tokenKeyMaxAge
}

func (k *tokenKeys) run(ctx context.Context) {
	ticker := k.clock.NewTicker(tokenKeyRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if err := k.update(ctx, false); err != nil && ctx.Err() == nil {
				ucplog.FromContextOrDiscard(ctx).Error(err, "cannot refresh cluster signing keys")
			}
		}
	}
}

// clusterTokenKeyFetcher uses only the configured API server and its Kubernetes TLS
// trust. Discovery's jwks_uri and token header URLs are never network destinations.
func clusterTokenKeyFetcher(config *rest.Config) (tokenKeyFetcher, error) {
	if config == nil || config.Insecure {
		return nil, errors.New("external identity requires verified Kubernetes API server TLS")
	}
	base, err := url.Parse(config.Host)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil ||
		base.RawQuery != "" || base.Fragment != "" {
		return nil, errors.New("external identity requires an HTTPS Kubernetes API server")
	}
	copy := rest.CopyConfig(config)
	copy.Timeout = tokenDiscoveryTimeout
	client, err := rest.HTTPClientFor(copy)
	if err != nil {
		return nil, errors.New("cannot initialize cluster signing key client")
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("cluster signing key discovery redirects are not allowed")
	}
	get := func(ctx context.Context, path string, out any) error {
		endpoint := *base
		endpoint.Path, endpoint.RawPath = path, ""
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return errors.New("cannot construct cluster signing key request")
		}
		resp, err := client.Do(req)
		if err != nil {
			return errors.New("cluster signing key discovery request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errors.New("cluster signing key discovery returned a non-success status")
		}
		const maxBody = 1024 * 1024
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
		if err != nil || len(body) > maxBody || json.Unmarshal(body, out) != nil {
			return errors.New("cluster signing key discovery returned invalid JSON")
		}
		return nil
	}
	return func(ctx context.Context) (*tokenKeySet, error) {
		var discovery struct {
			Issuer  string `json:"issuer"`
			JWKSURI string `json:"jwks_uri"`
		}
		if err := get(ctx, "/.well-known/openid-configuration", &discovery); err != nil {
			return nil, err
		}
		if !validIssuerURL(discovery.Issuer) || !validIssuerURL(discovery.JWKSURI) {
			return nil, errors.New("cluster signing key discovery has invalid issuer or JWKS URI")
		}
		var jwks struct {
			Keys []json.RawMessage `json:"keys"`
		}
		if err := get(ctx, "/openid/v1/jwks", &jwks); err != nil {
			return nil, err
		}
		if len(jwks.Keys) == 0 || len(jwks.Keys) > 100 {
			return nil, errors.New("cluster signing key set has invalid size")
		}
		set := &tokenKeySet{issuer: discovery.Issuer, keys: map[string]tokenSigningKey{}}
		for _, raw := range jwks.Keys {
			id, key, err := parseTokenKey(raw)
			if err != nil {
				return nil, err
			}
			if _, duplicate := set.keys[id]; duplicate {
				return nil, errors.New("cluster signing key set contains duplicate key IDs")
			}
			set.keys[id] = key
		}
		return set, nil
	}, nil
}

func validIssuerURL(text string) bool {
	u, err := url.Parse(text)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}

func parseTokenKey(raw json.RawMessage) (string, tokenSigningKey, error) {
	var jwk struct {
		KID string          `json:"kid"`
		KTY string          `json:"kty"`
		ALG string          `json:"alg"`
		Use string          `json:"use"`
		Ops []string        `json:"key_ops"`
		N   string          `json:"n"`
		E   string          `json:"e"`
		CRV string          `json:"crv"`
		X   string          `json:"x"`
		Y   string          `json:"y"`
		D   json.RawMessage `json:"d"`
		K   json.RawMessage `json:"k"`
	}
	invalid := errors.New("cluster signing key set contains an invalid or unsupported key")
	if json.Unmarshal(raw, &jwk) != nil || jwk.KID == "" || len(jwk.KID) > 256 ||
		(jwk.Use != "" && jwk.Use != "sig") || len(jwk.D) != 0 || len(jwk.K) != 0 {
		return "", tokenSigningKey{}, invalid
	}
	if len(jwk.Ops) > 0 && (len(jwk.Ops) != 1 || jwk.Ops[0] != "verify") {
		return "", tokenSigningKey{}, invalid
	}
	decode := func(s string) ([]byte, bool) {
		b, err := base64.RawURLEncoding.Strict().DecodeString(s)
		return b, err == nil && len(b) > 0
	}
	switch jwk.KTY {
	case "RSA":
		n, nok := decode(jwk.N)
		e, eok := decode(jwk.E)
		modulus := new(big.Int).SetBytes(n)
		exponent := new(big.Int).SetBytes(e)
		if !nok || !eok || modulus.BitLen() < 2048 || modulus.BitLen() > 8192 ||
			exponent.BitLen() > 31 || exponent.Int64() < 3 || exponent.Int64()%2 == 0 ||
			(jwk.ALG != "" && jwk.ALG != "RS256") {
			return "", tokenSigningKey{}, invalid
		}
		return jwk.KID, tokenSigningKey{algorithm: "RS256", public: &rsa.PublicKey{N: modulus, E: int(exponent.Int64())}}, nil
	case "EC":
		var curve elliptic.Curve
		algorithm := ""
		switch jwk.CRV {
		case "P-256":
			curve, algorithm = elliptic.P256(), "ES256"
		case "P-384":
			curve, algorithm = elliptic.P384(), "ES384"
		default:
			return "", tokenSigningKey{}, invalid
		}
		x, xok := decode(jwk.X)
		y, yok := decode(jwk.Y)
		key := &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !xok || !yok || len(x) != (curve.Params().BitSize+7)/8 || len(y) != len(x) ||
			!curve.IsOnCurve(key.X, key.Y) || (jwk.ALG != "" && jwk.ALG != algorithm) {
			return "", tokenSigningKey{}, invalid
		}
		return jwk.KID, tokenSigningKey{algorithm: algorithm, public: key}, nil
	default:
		return "", tokenSigningKey{}, invalid
	}
}
