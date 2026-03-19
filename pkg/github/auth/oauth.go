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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OAuthConfig holds the GitHub OAuth App configuration (used for user authentication).
type OAuthConfig struct {
	// ClientID is the OAuth App client ID.
	ClientID string

	// ClientSecret is the OAuth App client secret.
	ClientSecret string

	// RedirectURL is the URL GitHub redirects to after authorization.
	RedirectURL string

	// Scopes are the OAuth scopes to request.
	Scopes []string
}

// UserToken represents an authenticated GitHub user's token.
type UserToken struct {
	// AccessToken is the OAuth access token.
	AccessToken string

	// Login is the GitHub username.
	Login string

	// ExpiresAt is when the token expires (zero value means no expiry).
	ExpiresAt time.Time
}

// AuthorizationURL returns the GitHub OAuth authorization URL with a CSRF state parameter.
func (c *OAuthConfig) AuthorizationURL(state string) string {
	params := url.Values{
		"client_id":    {c.ClientID},
		"redirect_uri": {c.RedirectURL},
		"state":        {state},
	}
	if len(c.Scopes) > 0 {
		params.Set("scope", strings.Join(c.Scopes, " "))
	}
	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for an access token.
func (c *OAuthConfig) ExchangeCode(ctx context.Context, code string) (*UserToken, error) {
	payload := url.Values{
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
		"code":          {code},
		"redirect_uri":  {c.RedirectURL},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(payload.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token exchange request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("OAuth error: %s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Fetch the user's login name.
	login, err := fetchUserLogin(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, err
	}

	return &UserToken{
		AccessToken: tokenResp.AccessToken,
		Login:       login,
	}, nil
}

// fetchUserLogin retrieves the authenticated user's login from the GitHub API.
func fetchUserLogin(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("failed to parse user response: %w", err)
	}
	return user.Login, nil
}

// GenerateState generates a cryptographically random state string for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SessionStore provides in-memory session storage for user tokens.
// In production this should be replaced with a persistent store.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*UserToken
}

// NewSessionStore creates a new in-memory session store.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*UserToken),
	}
}

// Set stores a user token by session ID.
func (s *SessionStore) Set(sessionID string, token *UserToken) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = token
}

// Get retrieves a user token by session ID. Returns nil if not found.
func (s *SessionStore) Get(sessionID string) *UserToken {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// Delete removes a session.
func (s *SessionStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
}
