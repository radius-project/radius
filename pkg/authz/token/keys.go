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
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"

	"k8s.io/client-go/rest"
)

const (
	// discoveryPath is the API server's OIDC discovery document.
	discoveryPath = "/.well-known/openid-configuration"

	// apiServerJWKSPath is where the Kubernetes API server always serves its
	// ServiceAccount signing keys.
	apiServerJWKSPath = "/openid/v1/jwks"

	// maxResponseBytes bounds discovery and key set responses.
	maxResponseBytes = 1 << 20

	// minRSABits is the smallest RSA signing key accepted.
	minRSABits = 2048
)

// KeySet is a snapshot of the cluster's ServiceAccount token issuer and signing keys.
type KeySet struct {
	// Issuer is the "iss" value that tokens from the cluster carry.
	Issuer string

	// Keys maps each key ID to its public key, an *rsa.PublicKey or *ecdsa.PublicKey.
	Keys map[string]crypto.PublicKey
}

// KeySource fetches the cluster's current KeySet.
type KeySource interface {
	// Fetch returns the current KeySet. It must return an error rather than an empty
	// or partial KeySet when the keys cannot be read.
	Fetch(ctx context.Context) (KeySet, error)
}

// OIDCKeySource reads the KeySet from the Kubernetes API server's OIDC discovery
// endpoint and the key set at its jwks_uri.
type OIDCKeySource struct {
	// Client sends requests to the API server. It carries the API server's CA and the
	// component's credentials.
	Client *http.Client

	// BaseURL is the API server URL, such as "https://kubernetes.default.svc".
	BaseURL string
}

// NewOIDCKeySource returns an OIDCKeySource that uses the API server and credentials in config.
func NewOIDCKeySource(config *rest.Config) (*OIDCKeySource, error) {
	client, err := rest.HTTPClientFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create API server client: %w", err)
	}
	return &OIDCKeySource{Client: client, BaseURL: config.Host}, nil
}

type discoveryDocument struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

// Fetch reads the discovery document and then the key set it names. The key set is
// read through Client only when jwks_uri is on the API server. Otherwise, for example
// when the cluster publishes its keys at a public URL, Fetch reads the same keys from
// the API server's own key set path, so Client's credentials are never sent to another host.
func (s *OIDCKeySource) Fetch(ctx context.Context) (KeySet, error) {
	base, err := url.Parse(s.BaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return KeySet{}, fmt.Errorf("API server URL %q must be an absolute https URL", s.BaseURL)
	}

	var doc discoveryDocument
	if err := s.getJSON(ctx, base.JoinPath(discoveryPath).String(), &doc); err != nil {
		return KeySet{}, fmt.Errorf("failed to read OIDC discovery document: %w", err)
	}
	if doc.Issuer == "" {
		return KeySet{}, errors.New("OIDC discovery document has no issuer")
	}

	jwksURL, err := s.jwksURL(base, doc.JWKSURI)
	if err != nil {
		return KeySet{}, err
	}

	var set jsonWebKeySet
	if err := s.getJSON(ctx, jwksURL, &set); err != nil {
		return KeySet{}, fmt.Errorf("failed to read signing keys: %w", err)
	}
	keys, err := set.publicKeys()
	if err != nil {
		return KeySet{}, err
	}
	return KeySet{Issuer: doc.Issuer, Keys: keys}, nil
}

func (s *OIDCKeySource) jwksURL(base *url.URL, jwksURI string) (string, error) {
	if jwksURI == "" {
		return "", errors.New("OIDC discovery document has no jwks_uri")
	}
	u, err := url.Parse(jwksURI)
	if err != nil {
		return "", fmt.Errorf("invalid jwks_uri: %w", err)
	}
	if u.Scheme == base.Scheme && strings.EqualFold(u.Host, base.Host) {
		return u.String(), nil
	}
	return base.JoinPath(apiServerJWKSPath).String(), nil
}

func (s *OIDCKeySource) getJSON(ctx context.Context, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	client := http.Client{}
	if s.Client != nil {
		client = *s.Client
	}
	// Client may add credentials to every request, so never follow a redirect to another URL.
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s returned status %d", target, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxResponseBytes {
		return fmt.Errorf("GET %s returned more than %d bytes", target, maxResponseBytes)
	}
	return json.Unmarshal(body, out)
}

type jsonWebKeySet struct {
	Keys []jsonWebKey `json:"keys"`
}

type jsonWebKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// publicKeys returns the signing keys in the set by key ID. Keys without an ID, keys
// for another use, and key types or sizes that Radius does not accept are skipped. A
// set with no usable keys or with a repeated key ID is an error.
func (s jsonWebKeySet) publicKeys() (map[string]crypto.PublicKey, error) {
	keys := map[string]crypto.PublicKey{}
	for _, k := range s.Keys {
		if k.Kid == "" || (k.Use != "" && k.Use != "sig") {
			continue
		}
		if _, ok := keys[k.Kid]; ok {
			return nil, fmt.Errorf("key set has more than one key with ID %q", k.Kid)
		}
		key, err := k.publicKey()
		if err != nil {
			continue
		}
		keys[k.Kid] = key
	}
	if len(keys) == 0 {
		return nil, errors.New("key set has no usable signing keys")
	}
	return keys, nil
}

// publicKey accepts RSA keys of at least 2048 bits and ECDSA P-256 or P-384 keys.
func (k jsonWebKey) publicKey() (crypto.PublicKey, error) {
	switch k.Kty {
	case "RSA":
		n, err := decodeBase64URL(k.N)
		if err != nil {
			return nil, err
		}
		e, err := decodeBase64URL(k.E)
		if err != nil {
			return nil, err
		}
		exponent := new(big.Int).SetBytes(e)
		if !exponent.IsInt64() || exponent.Int64() < 3 || exponent.Int64() > 1<<31-1 {
			return nil, errors.New("invalid RSA exponent")
		}
		key := &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent.Int64())}
		if key.N.BitLen() < minRSABits {
			return nil, fmt.Errorf("RSA key size %d is below %d", key.N.BitLen(), minRSABits)
		}
		return key, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		default:
			return nil, fmt.Errorf("curve %q is not allowed", k.Crv)
		}
		x, err := decodeBase64URL(k.X)
		if err != nil {
			return nil, err
		}
		y, err := decodeBase64URL(k.Y)
		if err != nil {
			return nil, err
		}
		size := (curve.Params().BitSize + 7) / 8
		if len(x) != size || len(y) != size {
			return nil, errors.New("invalid EC point size")
		}
		return ecdsa.ParseUncompressedPublicKey(curve, append(append([]byte{4}, x...), y...))
	default:
		return nil, fmt.Errorf("key type %q is not allowed", k.Kty)
	}
}

func decodeBase64URL(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New("missing key parameter")
	}
	return base64.RawURLEncoding.DecodeString(s)
}
