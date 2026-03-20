/*
Copyright 2023 The Radius Authors.

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

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return key
}

func marshalRSAPrivateKeyPEM(key *rsa.PrivateKey) []byte {
	derBytes := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: derBytes,
	})
}

func TestGenerateJWT(t *testing.T) {
	key := generateTestKey(t)
	appAuth := NewAppAuth(AppConfig{
		AppID:          12345,
		PrivateKey:     key,
		InstallationID: 67890,
	})

	token, err := appAuth.GenerateJWT()
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// The JWT should have 3 parts (header.payload.signature).
	parts := strings.Split(token, ".")
	assert.Len(t, parts, 3)
}

func TestGetInstallationToken(t *testing.T) {
	key := generateTestKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "/app/installations/67890/access_tokens")
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"token": "ghs_test_token"}) //nolint:errcheck
	}))
	defer server.Close()

	appAuth := NewAppAuthWithOptions(
		AppConfig{
			AppID:          12345,
			PrivateKey:     key,
			InstallationID: 67890,
		},
		server.Client(),
		server.URL,
	)

	token, err := appAuth.GetInstallationToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ghs_test_token", token)
}

func TestGetInstallationToken_Error(t *testing.T) {
	key := generateTestKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`)) //nolint:errcheck
	}))
	defer server.Close()

	appAuth := NewAppAuthWithOptions(
		AppConfig{
			AppID:          12345,
			PrivateKey:     key,
			InstallationID: 67890,
		},
		server.Client(),
		server.URL,
	)

	_, err := appAuth.GetInstallationToken(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestTokenSource(t *testing.T) {
	key := generateTestKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"token": "ghs_from_source"}) //nolint:errcheck
	}))
	defer server.Close()

	appAuth := NewAppAuthWithOptions(
		AppConfig{
			AppID:          12345,
			PrivateKey:     key,
			InstallationID: 67890,
		},
		server.Client(),
		server.URL,
	)

	ts := appAuth.TokenSource()
	token, err := ts.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ghs_from_source", token)
}

func TestParsePrivateKey(t *testing.T) {
	key := generateTestKey(t)
	pemData := marshalRSAPrivateKeyPEM(key)

	parsed, err := ParsePrivateKey(pemData)
	require.NoError(t, err)
	assert.NotNil(t, parsed)
}

func TestParsePrivateKey_Invalid(t *testing.T) {
	_, err := ParsePrivateKey([]byte("not a pem"))
	assert.Error(t, err)
}

func TestRequireAuth_Unauthorized(t *testing.T) {
	store := NewSessionStore()
	handler := RequireAuth(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAuth_BearerToken(t *testing.T) {
	store := NewSessionStore()
	store.Set("session-123", &UserToken{Login: "testuser", AccessToken: "tok"})

	var capturedUser *UserToken
	handler := RequireAuth(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer session-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedUser)
	assert.Equal(t, "testuser", capturedUser.Login)
}

func TestRequireAuth_Cookie(t *testing.T) {
	store := NewSessionStore()
	store.Set("cookie-session", &UserToken{Login: "cookieuser", AccessToken: "tok"})

	handler := RequireAuth(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		assert.Equal(t, "cookieuser", user.Login)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-session"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()

	assert.Nil(t, store.Get("nonexistent"))

	token := &UserToken{Login: "user1", AccessToken: "abc"}
	store.Set("s1", token)
	assert.Equal(t, token, store.Get("s1"))

	store.Delete("s1")
	assert.Nil(t, store.Get("s1"))
}

func TestGenerateState(t *testing.T) {
	state, err := GenerateState()
	require.NoError(t, err)
	assert.Len(t, state, 32) // 16 bytes = 32 hex chars
}

func TestOAuthConfig_AuthorizationURL(t *testing.T) {
	cfg := &OAuthConfig{
		ClientID:    "client-id",
		RedirectURL: "http://localhost:8080/callback",
		Scopes:      []string{"repo", "read:org"},
	}

	url := cfg.AuthorizationURL("test-state")
	assert.Contains(t, url, "client_id=client-id")
	assert.Contains(t, url, "state=test-state")
	assert.Contains(t, url, "redirect_uri=")
	assert.Contains(t, url, "scope=repo+read%3Aorg")
}
