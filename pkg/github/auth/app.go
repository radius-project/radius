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
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/radius-project/radius/pkg/github/environment"
)

const (
	githubAPIBaseURL = "https://api.github.com"
)

// AppConfig holds the GitHub App configuration.
type AppConfig struct {
	// AppID is the GitHub App's numeric ID.
	AppID int64

	// PrivateKey is the RSA private key used to sign JWTs.
	PrivateKey *rsa.PrivateKey

	// InstallationID is the installation ID for the target organization/repo.
	InstallationID int64
}

// AppAuth handles GitHub App authentication: JWT generation and installation
// token exchange.
type AppAuth struct {
	config     AppConfig
	httpClient *http.Client
	baseURL    string
}

// NewAppAuth creates a new GitHub App authenticator.
func NewAppAuth(config AppConfig) *AppAuth {
	return &AppAuth{
		config:     config,
		httpClient: http.DefaultClient,
		baseURL:    githubAPIBaseURL,
	}
}

// NewAppAuthWithOptions creates a new GitHub App authenticator with a custom HTTP
// client and base URL. Primarily for testing.
func NewAppAuthWithOptions(config AppConfig, httpClient *http.Client, baseURL string) *AppAuth {
	return &AppAuth{
		config:     config,
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// GenerateJWT creates a short-lived JWT signed with the App's private key.
// The JWT is used to authenticate as the GitHub App itself (not as an installation).
func (a *AppAuth) GenerateJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)), // clock drift buffer
		ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		Issuer:    fmt.Sprintf("%d", a.config.AppID),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(a.config.PrivateKey)
}

// GetInstallationToken exchanges the App JWT for an installation access token.
// This token is used to perform actions on behalf of the installation (read/write
// environment variables, secrets, etc.).
func (a *AppAuth) GetInstallationToken(ctx context.Context) (string, error) {
	appJWT, err := a.GenerateJWT()
	if err != nil {
		return "", fmt.Errorf("failed to generate app JWT: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.baseURL, a.config.InstallationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read installation token response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("installation token request failed with status %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse installation token response: %w", err)
	}

	return result.Token, nil
}

// TokenSource returns an environment.TokenSource backed by GitHub App installation
// tokens. Each call to Token() generates a fresh installation token.
func (a *AppAuth) TokenSource() environment.TokenSource {
	return &appTokenSource{appAuth: a}
}

type appTokenSource struct {
	appAuth *AppAuth
}

func (s *appTokenSource) Token(ctx context.Context) (string, error) {
	return s.appAuth.GetInstallationToken(ctx)
}

// ParsePrivateKey parses a PEM-encoded RSA private key.
func ParsePrivateKey(pemData []byte) (*rsa.PrivateKey, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA private key: %w", err)
	}
	return key, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
