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
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
)

const (
	testIssuer    = "https://kubernetes.default.svc.cluster.local"
	testNamespace = "radius-system"
	testAudience  = "applications-rp"
)

var (
	rsaKey = mustRSAKey()
	ecKey  = mustECKey()
	start  = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
)

func mustRSAKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

func mustECKey() *ecdsa.PrivateKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return k
}

// fakeClock is a settable clock.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeSource returns a settable KeySet or error and counts fetches.
type fakeSource struct {
	mu      sync.Mutex
	keys    map[string]crypto.PublicKey
	err     error
	fetches int
}

func newFakeSource(keys map[string]crypto.PublicKey) *fakeSource {
	return &fakeSource{keys: keys}
}

func (s *fakeSource) Fetch(context.Context) (KeySet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fetches++
	if s.err != nil {
		return KeySet{}, s.err
	}
	return KeySet{Issuer: testIssuer, Keys: maps.Clone(s.keys)}, nil
}

func (s *fakeSource) set(keys map[string]crypto.PublicKey, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys = keys
	s.err = err
}

func (s *fakeSource) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fetches
}

func defaultKeys() map[string]crypto.PublicKey {
	return map[string]crypto.PublicKey{"rsa": &rsaKey.PublicKey, "ec": &ecKey.PublicKey}
}

func newTestVerifier(t *testing.T, source KeySource, clock *fakeClock) *Verifier {
	t.Helper()
	v, err := New(Config{Namespace: testNamespace, Audience: testAudience, Keys: source, Now: clock.Now})
	require.NoError(t, err)
	return v
}

// claimsFor returns valid claims for a ServiceAccount in the Radius namespace.
func claimsFor(account string, now time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": testIssuer,
		"aud": []string{testAudience},
		"sub": "system:serviceaccount:" + testNamespace + ":" + account,
		"iat": now.Unix(),
		"nbf": now.Unix(),
		"exp": now.Add(10 * time.Minute).Unix(),
		"kubernetes.io": map[string]any{
			"namespace":      testNamespace,
			"serviceaccount": map[string]any{"name": account, "uid": "1234"},
		},
	}
}

func sign(t *testing.T, method jwt.SigningMethod, kid string, key any, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var tokenErr *Error
	require.ErrorAs(t, err, &tokenErr)
	require.Equal(t, code, tokenErr.Code)
}

func TestNew_Validation(t *testing.T) {
	source := newFakeSource(defaultKeys())
	tests := []struct {
		name   string
		config Config
	}{
		{name: "no namespace", config: Config{Audience: testAudience, Keys: source}},
		{name: "no audience", config: Config{Namespace: testNamespace, Keys: source}},
		{name: "no keys", config: Config{Namespace: testNamespace, Audience: testAudience}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)
			require.Error(t, err)
		})
	}
}

func TestServiceAccountsMatchComponents(t *testing.T) {
	require.ElementsMatch(t, mtls.Components, slices.Collect(maps.Keys(serviceForAccount)))
}

func TestVerify_Valid(t *testing.T) {
	clock := &fakeClock{now: start}
	v := newTestVerifier(t, newFakeSource(defaultKeys()), clock)
	require.NoError(t, v.Refresh(t.Context()))

	for _, account := range mtls.Components {
		t.Run(account+"/RS256", func(t *testing.T) {
			id, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor(account, start)))
			require.NoError(t, err)
			require.Equal(t, mtls.Identity{Component: account, Namespace: testNamespace}, id)
			require.Equal(t, account+"."+testNamespace+".svc", id.String())
		})
		t.Run(account+"/ES256", func(t *testing.T) {
			id, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodES256, "ec", ecKey, claimsFor(account, start)))
			require.NoError(t, err)
			require.Equal(t, account, id.Component)
		})
	}
}

func TestVerify_Invalid(t *testing.T) {
	otherKey := mustRSAKey()
	tests := []struct {
		name  string
		token func(t *testing.T) string
	}{
		{name: "empty", token: func(*testing.T) string { return "" }},
		{name: "malformed", token: func(*testing.T) string { return "not-a-jwt" }},
		{name: "wrong audience", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			c["aud"] = []string{"dynamic-rp"}
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "no audience", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			delete(c, "aud")
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "wrong issuer", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			c["iss"] = "https://other.example.com"
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "expired", token: func(t *testing.T) string {
			c := claimsFor("ucp", start.Add(-time.Hour))
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "no expiry", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			delete(c, "exp")
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "no not-before", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			delete(c, "nbf")
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "not yet valid", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			c["nbf"] = start.Add(5 * time.Minute).Unix()
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "wrong namespace", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			c["sub"] = "system:serviceaccount:default:ucp"
			c["kubernetes.io"] = map[string]any{"namespace": "default", "serviceaccount": map[string]any{"name": "ucp"}}
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "ServiceAccount outside allowlist", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("dashboard", start))
		}},
		{name: "default ServiceAccount", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("default", start))
		}},
		{name: "subject does not match ServiceAccount", token: func(t *testing.T) string {
			c := claimsFor("ucp", start)
			c["sub"] = "system:serviceaccount:" + testNamespace + ":dashboard"
			return sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c)
		}},
		{name: "no key ID", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, "", rsaKey, claimsFor("ucp", start))
		}},
		{name: "signed by another key", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, "rsa", otherKey, claimsFor("ucp", start))
		}},
		{name: "key type does not match algorithm", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, "ec", rsaKey, claimsFor("ucp", start))
		}},
		{name: "HMAC algorithm", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS256, "rsa", []byte("secret"), claimsFor("ucp", start))
		}},
		{name: "none algorithm", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodNone, "rsa", jwt.UnsafeAllowNoneSignatureType, claimsFor("ucp", start))
		}},
		{name: "RS512 algorithm", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS512, "rsa", rsaKey, claimsFor("ucp", start))
		}},
	}

	clock := &fakeClock{now: start}
	v := newTestVerifier(t, newFakeSource(defaultKeys()), clock)
	require.NoError(t, v.Refresh(t.Context()))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.Verify(t.Context(), tt.token(t))
			requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
			require.Equal(t, http.StatusUnauthorized, authz.StatusForCode(err.(*Error).Code))
		})
	}
}

func TestVerify_Leeway(t *testing.T) {
	clock := &fakeClock{now: start}
	v := newTestVerifier(t, newFakeSource(defaultKeys()), clock)
	require.NoError(t, v.Refresh(t.Context()))

	c := claimsFor("ucp", start)
	c["nbf"] = start.Add(30 * time.Second).Unix()
	_, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c))
	require.NoError(t, err, "nbf within leeway")

	clock.Advance(10*time.Minute + 30*time.Second)
	token := sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start))
	_, err = v.Verify(t.Context(), token)
	require.NoError(t, err, "exp within leeway")

	clock.Advance(time.Minute)
	_, err = v.Verify(t.Context(), token)
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
}

func TestVerify_ConfiguredIssuer(t *testing.T) {
	clock := &fakeClock{now: start}
	v, err := New(Config{Namespace: testNamespace, Audience: testAudience, Keys: newFakeSource(defaultKeys()), Now: clock.Now, Issuer: "https://configured"})
	require.NoError(t, err)
	require.NoError(t, v.Refresh(t.Context()))

	_, err = v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start)))
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)

	c := claimsFor("ucp", start)
	c["iss"] = "https://configured"
	_, err = v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, c))
	require.NoError(t, err)
}

func TestVerifyRequest(t *testing.T) {
	clock := &fakeClock{now: start}
	v := newTestVerifier(t, newFakeSource(defaultKeys()), clock)
	require.NoError(t, v.Refresh(t.Context()))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer "+sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start)))
	id, err := v.VerifyRequest(r)
	require.NoError(t, err)
	require.Equal(t, "ucp."+testNamespace+".svc", id.String())

	_, err = v.VerifyRequest(httptest.NewRequest(http.MethodGet, "/", nil))
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
}

func TestVerify_NotLoaded(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v := newTestVerifier(t, source, clock)

	_, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start)))
	requireCode(t, err, v1.CodeAuthorizationUnavailable)
	require.Equal(t, http.StatusServiceUnavailable, authz.StatusForCode(err.(*Error).Code))
	require.Zero(t, source.count(), "an unloaded verifier does not fetch on the request path")
}

func TestVerify_UnknownKeyRefresh(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(map[string]crypto.PublicKey{"rsa": &rsaKey.PublicKey})
	v := newTestVerifier(t, source, clock)
	require.NoError(t, v.Refresh(t.Context()))
	require.Equal(t, 1, source.count())

	// The cluster adds a new signing key after the last refresh.
	source.set(defaultKeys(), nil)

	id, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodES256, "ec", ecKey, claimsFor("controller", start)))
	require.NoError(t, err)
	require.Equal(t, "controller", id.Component)
	require.Equal(t, 2, source.count())

	// The new key is cached, so verifying again does not refetch.
	_, err = v.Verify(t.Context(), sign(t, jwt.SigningMethodES256, "ec", ecKey, claimsFor("controller", start)))
	require.NoError(t, err)
	require.Equal(t, 2, source.count())
}

func TestVerify_UnknownKeyRateLimit(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v := newTestVerifier(t, source, clock)
	require.NoError(t, v.Refresh(t.Context()))

	unknown := sign(t, jwt.SigningMethodRS256, "missing", rsaKey, claimsFor("ucp", start))

	_, err := v.Verify(t.Context(), unknown)
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
	require.Equal(t, 2, source.count())

	for range 5 {
		_, err = v.Verify(t.Context(), unknown)
		requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
	}
	clock.Advance(DefaultUnknownKeyInterval - time.Second)
	_, err = v.Verify(t.Context(), unknown)
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
	require.Equal(t, 2, source.count(), "refetches for unknown key IDs are rate limited")

	clock.Advance(time.Second)
	_, err = v.Verify(t.Context(), unknown)
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
	require.Equal(t, 3, source.count())
}

func TestVerify_UnknownKeyRateLimitConcurrent(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v := newTestVerifier(t, source, clock)
	require.NoError(t, v.Refresh(t.Context()))

	unknown := sign(t, jwt.SigningMethodRS256, "missing", rsaKey, claimsFor("ucp", start))
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			_, err := v.Verify(t.Context(), unknown)
			requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
		})
	}
	wg.Wait()
	require.Equal(t, 2, source.count())
}

func TestVerify_RemovedKey(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v := newTestVerifier(t, source, clock)
	require.NoError(t, v.Refresh(t.Context()))

	token := sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start))
	_, err := v.Verify(t.Context(), token)
	require.NoError(t, err)

	// The cluster removes the RSA key; the next periodic refresh drops it.
	source.set(map[string]crypto.PublicKey{"ec": &ecKey.PublicKey}, nil)
	clock.Advance(DefaultRefreshInterval)
	require.NoError(t, v.Refresh(t.Context()))

	_, err = v.Verify(t.Context(), token)
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)
}

func TestRun_RemovesKeyWithinRefreshInterval(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v, err := New(Config{
		Namespace:       testNamespace,
		Audience:        testAudience,
		Keys:            source,
		Now:             clock.Now,
		RefreshInterval: 10 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		v.Run(ctx)
	}()

	token := sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start))
	require.Eventually(t, func() bool {
		_, err := v.Verify(t.Context(), token)
		return err == nil
	}, 5*time.Second, 5*time.Millisecond)

	source.set(map[string]crypto.PublicKey{"ec": &ecKey.PublicKey}, nil)
	before := source.count()
	require.Eventually(t, func() bool { return source.count() >= before+2 }, 5*time.Second, 5*time.Millisecond)
	clock.Advance(DefaultUnknownKeyInterval)

	_, err = v.Verify(t.Context(), token)
	requireCode(t, err, v1.CodeInvalidAuthenticationInfo)

	cancel()
	<-done
}

func TestRun_RetriesAfterFailure(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	source.set(defaultKeys(), errors.New("API server unavailable"))
	v, err := New(Config{
		Namespace:          testNamespace,
		Audience:           testAudience,
		Keys:               source,
		Now:                clock.Now,
		RefreshInterval:    time.Hour,
		UnknownKeyInterval: 10 * time.Millisecond,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	go v.Run(ctx)

	require.Eventually(t, func() bool { return source.count() >= 2 }, 5*time.Second, 5*time.Millisecond)
	source.set(defaultKeys(), nil)

	token := sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start))
	require.Eventually(t, func() bool {
		_, err := v.Verify(t.Context(), token)
		return err == nil
	}, 5*time.Second, 5*time.Millisecond)
}

func TestVerify_FailsClosedWhenKeysAreStale(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v := newTestVerifier(t, source, clock)
	require.NoError(t, v.Refresh(t.Context()))

	source.set(defaultKeys(), errors.New("API server unavailable"))
	for range 3 {
		clock.Advance(DefaultRefreshInterval)
		require.Error(t, v.Refresh(t.Context()))
	}
	require.Equal(t, DefaultMaxKeyAge, clock.Now().Sub(start))

	// Just under MaxKeyAge since the last success: still accepted.
	clock.Advance(-time.Nanosecond)
	_, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", clock.Now())))
	require.NoError(t, err)

	// Exactly MaxKeyAge since the last success: stale.
	clock.Advance(time.Nanosecond)
	_, err = v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", clock.Now())))
	requireCode(t, err, v1.CodeAuthorizationUnavailable)
	require.Equal(t, http.StatusServiceUnavailable, authz.StatusForCode(err.(*Error).Code))

	// A token with an unknown key ID does not bypass the stale check.
	_, err = v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "missing", rsaKey, claimsFor("ucp", clock.Now())))
	requireCode(t, err, v1.CodeAuthorizationUnavailable)

	// A successful refresh restores verification.
	source.set(defaultKeys(), nil)
	require.NoError(t, v.Refresh(t.Context()))
	_, err = v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", clock.Now())))
	require.NoError(t, err)
}

func TestRefresh_KeepsKeysOnFailure(t *testing.T) {
	clock := &fakeClock{now: start}
	source := newFakeSource(defaultKeys())
	v := newTestVerifier(t, source, clock)
	require.NoError(t, v.Refresh(t.Context()))

	source.set(nil, nil)
	require.Error(t, v.Refresh(t.Context()), "an empty key set is a failed refresh")

	_, err := v.Verify(t.Context(), sign(t, jwt.SigningMethodRS256, "rsa", rsaKey, claimsFor("ucp", start)))
	require.NoError(t, err)
}

func TestError(t *testing.T) {
	inner := errors.New("inner")
	err := invalid("reason", inner)
	require.Equal(t, "InvalidAuthenticationInfo: reason: inner", err.Error())
	require.ErrorIs(t, err, inner)
	require.Equal(t, "AuthorizationUnavailable: reason", unavailable("reason", nil).Error())
}
