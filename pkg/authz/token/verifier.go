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
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
)

const (
	// DefaultRefreshInterval is how often the signing keys are refetched.
	DefaultRefreshInterval = 5 * time.Minute

	// DefaultUnknownKeyInterval is the shortest time between refetches caused by a
	// token with an unknown key ID. It is also the retry interval after a failed refresh.
	DefaultUnknownKeyInterval = 30 * time.Second

	// DefaultMaxKeyAge is how long the keys may go without a successful refresh before
	// every token is rejected with AuthorizationUnavailable.
	DefaultMaxKeyAge = 15 * time.Minute

	// DefaultFetchTimeout bounds a single key fetch.
	DefaultFetchTimeout = 10 * time.Second

	// DefaultLeeway is the clock skew allowed when checking exp and nbf. It matches the
	// leeway the Kubernetes API server uses for ServiceAccount tokens.
	DefaultLeeway = time.Minute
)

// signingMethods are the algorithms accepted for the keys publicKey allows.
var signingMethods = []string{"RS256", "ES256", "ES384"}

// curveForMethod is the curve each ECDSA signing method requires.
var curveForMethod = map[string]elliptic.Curve{"ES256": elliptic.P256(), "ES384": elliptic.P384()}

// serviceForAccount maps each Radius ServiceAccount allowed to call another component to
// the Kubernetes Service name used in its canonical identity. The chart gives each
// component a ServiceAccount and a Service with the same name.
var serviceForAccount = map[string]string{
	"ucp":             "ucp",
	"applications-rp": "applications-rp",
	"dynamic-rp":      "dynamic-rp",
	"controller":      "controller",
	"bicep-de":        "bicep-de",
}

// Config configures a Verifier.
type Config struct {
	// Namespace is the Radius namespace. Only ServiceAccounts in this namespace are accepted.
	Namespace string

	// Audience is the receiving component's own name. Tokens must list it in "aud".
	Audience string

	// Keys fetches the cluster's issuer and signing keys.
	Keys KeySource

	// Issuer, if set, is the required "iss" value. Otherwise the issuer from Keys is used.
	Issuer string

	// Now returns the current time. Defaults to time.Now.
	Now func() time.Time

	// RefreshInterval defaults to DefaultRefreshInterval.
	RefreshInterval time.Duration

	// UnknownKeyInterval defaults to DefaultUnknownKeyInterval.
	UnknownKeyInterval time.Duration

	// MaxKeyAge defaults to DefaultMaxKeyAge.
	MaxKeyAge time.Duration

	// FetchTimeout defaults to DefaultFetchTimeout.
	FetchTimeout time.Duration

	// Leeway defaults to DefaultLeeway. Use a negative value for no leeway.
	Leeway time.Duration
}

// Verifier verifies Kubernetes ServiceAccount tokens sent by Radius components. Call Run
// to load and refresh the signing keys; until a load succeeds, Verify returns
// AuthorizationUnavailable. A Verifier is safe for concurrent use.
type Verifier struct {
	namespace          string
	audience           string
	issuer             string
	source             KeySource
	now                func() time.Time
	refreshInterval    time.Duration
	unknownKeyInterval time.Duration
	maxKeyAge          time.Duration
	fetchTimeout       time.Duration
	leeway             time.Duration

	// fetchMu serializes fetches and guards lastUnknownKeyFetch.
	fetchMu             sync.Mutex
	lastUnknownKeyFetch time.Time

	// mu guards keys and lastSuccess.
	mu          sync.RWMutex
	keys        KeySet
	lastSuccess time.Time
}

// New returns a Verifier for config. Namespace, Audience, and Keys are required. A zero
// duration uses its default.
func New(config Config) (*Verifier, error) {
	if config.Namespace == "" {
		return nil, errors.New("namespace is required")
	}
	if config.Audience == "" {
		return nil, errors.New("audience is required")
	}
	if config.Keys == nil {
		return nil, errors.New("key source is required")
	}

	v := &Verifier{
		namespace:          config.Namespace,
		audience:           config.Audience,
		issuer:             config.Issuer,
		source:             config.Keys,
		now:                config.Now,
		refreshInterval:    orDefault(config.RefreshInterval, DefaultRefreshInterval),
		unknownKeyInterval: orDefault(config.UnknownKeyInterval, DefaultUnknownKeyInterval),
		maxKeyAge:          orDefault(config.MaxKeyAge, DefaultMaxKeyAge),
		fetchTimeout:       orDefault(config.FetchTimeout, DefaultFetchTimeout),
		leeway:             orDefault(config.Leeway, DefaultLeeway),
	}
	if v.now == nil {
		v.now = time.Now
	}
	if v.leeway < 0 {
		v.leeway = 0
	}
	return v, nil
}

func orDefault(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

// Run loads the signing keys and refreshes them every RefreshInterval until ctx is done.
// When a Refresh already loaded the keys, the first refresh waits a full RefreshInterval.
// After a failed refresh it retries every UnknownKeyInterval.
func (v *Verifier) Run(ctx context.Context) {
	logger := ucplog.FromContextOrDiscard(ctx)
	v.mu.RLock()
	first := time.Duration(0)
	if !v.lastSuccess.IsZero() {
		first = v.refreshInterval
	}
	v.mu.RUnlock()
	timer := time.NewTimer(first)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		wait := v.refreshInterval
		if err := v.Refresh(ctx); err != nil {
			logger.Error(err, "Failed to refresh ServiceAccount token signing keys")
			wait = v.unknownKeyInterval
		}
		timer.Reset(wait)
	}
}

// Refresh fetches the signing keys and replaces the cached keys if the fetch succeeds.
func (v *Verifier) Refresh(ctx context.Context) error {
	v.fetchMu.Lock()
	defer v.fetchMu.Unlock()
	return v.fetchLocked(ctx)
}

// fetchLocked fetches the keys within FetchTimeout. The caller holds fetchMu. A
// failed fetch keeps the cached keys. A successful fetch replaces them with the keys
// Radius accepts. If there are none, it returns an error and Verify rejects every token
// with AuthorizationUnavailable, so a key the cluster removed is not trusted for longer.
func (v *Verifier) fetchLocked(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, v.fetchTimeout)
	defer cancel()
	keys, err := v.source.Fetch(ctx)
	if err != nil {
		return err
	}
	if keys.Issuer == "" {
		return errors.New("key source returned no issuer")
	}
	accepted := make(map[string]crypto.PublicKey, len(keys.Keys))
	for kid, key := range keys.Keys {
		if kid != "" && acceptedKey(key) {
			accepted[kid] = key
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	v.keys = KeySet{Issuer: keys.Issuer, Keys: accepted}
	v.lastSuccess = v.now()
	if len(accepted) == 0 {
		return errors.New("key set has no usable signing keys")
	}
	return nil
}

// refreshForUnknownKey refetches the keys unless a refetch for an unknown key ID ran
// within UnknownKeyInterval.
func (v *Verifier) refreshForUnknownKey(ctx context.Context) {
	v.fetchMu.Lock()
	defer v.fetchMu.Unlock()

	now := v.now()
	if !v.lastUnknownKeyFetch.IsZero() && now.Sub(v.lastUnknownKeyFetch) < v.unknownKeyInterval {
		return
	}
	v.lastUnknownKeyFetch = now
	if err := v.fetchLocked(ctx); err != nil {
		ucplog.FromContextOrDiscard(ctx).Error(err, "Failed to refresh ServiceAccount token signing keys for an unknown key ID")
	}
}

// current returns the cached keys, or an AuthorizationUnavailable error if they have
// never loaded, are at least MaxKeyAge old, or contain no usable key.
func (v *Verifier) current() (KeySet, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.lastSuccess.IsZero() {
		return KeySet{}, unavailable("signing keys have not been loaded", nil)
	}
	if v.now().Sub(v.lastSuccess) >= v.maxKeyAge {
		return KeySet{}, unavailable("signing keys could not be refreshed", nil)
	}
	if len(v.keys.Keys) == 0 {
		return KeySet{}, unavailable("the cluster publishes no usable signing keys", nil)
	}
	return v.keys, nil
}

// VerifyRequest verifies the bearer token in r's Authorization header. If the signing keys
// are not fresh, it returns AuthorizationUnavailable before it reads the header.
func (v *Verifier) VerifyRequest(r *http.Request) (authz.Identity, error) {
	if _, err := v.current(); err != nil {
		return authz.Identity{}, err
	}
	token, err := BearerToken(r)
	if err != nil {
		return authz.Identity{}, err
	}
	return v.Verify(r.Context(), token)
}

// Verify checks token's signature, issuer, audience (exactly one, this receiver), expiry,
// not-before time, namespace, and ServiceAccount, and returns the calling component's
// identity. Failures are *Error values: AuthorizationUnavailable if the signing keys are
// not fresh or none are usable, otherwise InvalidAuthenticationInfo.
func (v *Verifier) Verify(ctx context.Context, token string) (authz.Identity, error) {
	if _, err := v.current(); err != nil {
		return authz.Identity{}, err
	}
	if token == "" {
		return authz.Identity{}, invalid("missing bearer token", nil)
	}

	claims, unknownKey, err := v.parse(token)
	if unknownKey {
		v.refreshForUnknownKey(ctx)
		claims, unknownKey, err = v.parse(token)
	}
	if err != nil {
		var tokenErr *Error
		if errors.As(err, &tokenErr) {
			return authz.Identity{}, tokenErr
		}
		if unknownKey {
			return authz.Identity{}, invalid("token is signed with an unknown key", nil)
		}
		return authz.Identity{}, invalid("token is not valid", err)
	}
	return v.identity(claims)
}

// parse verifies token against the cached keys. unknownKey reports whether the token
// names a key ID that is not cached.
func (v *Verifier) parse(token string) (claims *serviceAccountClaims, unknownKey bool, err error) {
	keys, err := v.current()
	if err != nil {
		return nil, false, err
	}
	issuer := v.issuer
	if issuer == "" {
		issuer = keys.Issuer
	}

	parser := jwt.NewParser(
		jwt.WithValidMethods(signingMethods),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
		jwt.WithNotBeforeRequired(),
		jwt.WithLeeway(v.leeway),
		jwt.WithTimeFunc(v.now),
	)
	claims = &serviceAccountClaims{}
	_, err = parser.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, errors.New("token has no key ID")
		}
		key, ok := keys.Keys[kid]
		if !ok {
			unknownKey = true
			return nil, errors.New("unknown key ID")
		}
		// RFC 7518 section 3.4 binds ES256 to P-256 and ES384 to P-384; the JWT library checks
		// only the signature length.
		if ecKey, ok := key.(*ecdsa.PublicKey); ok {
			if want := curveForMethod[t.Method.Alg()]; want == nil || ecKey.Curve != want {
				return nil, fmt.Errorf("key %q does not use the curve that %s requires", kid, t.Method.Alg())
			}
		}
		return key, nil
	})
	if err != nil {
		return nil, unknownKey, err
	}
	return claims, false, nil
}

// identity maps verified claims to the calling component's identity.
func (v *Verifier) identity(claims *serviceAccountClaims) (authz.Identity, error) {
	// The parser checks that aud contains this receiver. A token for more than one
	// receiver could be replayed from one to another, so require exactly one.
	if len(claims.Audience) != 1 {
		return authz.Identity{}, invalid("token must name exactly one audience", nil)
	}
	k := claims.Kubernetes
	if k.Namespace != v.namespace {
		return authz.Identity{}, invalid(fmt.Sprintf("ServiceAccount namespace %q is not the Radius namespace", k.Namespace), nil)
	}
	service, ok := serviceForAccount[k.ServiceAccount.Name]
	if !ok {
		return authz.Identity{}, invalid(fmt.Sprintf("ServiceAccount %q is not a Radius component", k.ServiceAccount.Name), nil)
	}
	if want := "system:serviceaccount:" + k.Namespace + ":" + k.ServiceAccount.Name; claims.Subject != want {
		return authz.Identity{}, invalid("token subject does not match its ServiceAccount", nil)
	}
	return authz.Identity{Component: service, Namespace: v.namespace}, nil
}

// serviceAccountClaims are the claims in a Kubernetes ServiceAccount token.
type serviceAccountClaims struct {
	jwt.RegisteredClaims

	Kubernetes kubernetesClaims `json:"kubernetes.io"`
}

type kubernetesClaims struct {
	Namespace      string        `json:"namespace"`
	ServiceAccount namedResource `json:"serviceaccount"`
}

type namedResource struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}
