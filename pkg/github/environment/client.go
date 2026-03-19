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

package environment

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/crypto/nacl/box"
)

const (
	githubAPIBaseURL = "https://api.github.com"
)

// TokenSource provides GitHub API tokens. Implementations may return GitHub App
// installation tokens or user OAuth tokens.
type TokenSource interface {
	// Token returns a valid GitHub API token.
	Token(ctx context.Context) (string, error)
}

// Client manages GitHub Environment variables and secrets via the GitHub REST API.
type Client interface {
	// CreateEnvironment creates a GitHub Environment in the repository.
	CreateEnvironment(ctx context.Context, owner, repo, envName string) error

	// EnvironmentExists checks whether a GitHub Environment exists.
	EnvironmentExists(ctx context.Context, owner, repo, envName string) (bool, error)

	// ListEnvironments returns the names of all GitHub Environments in the repository.
	ListEnvironments(ctx context.Context, owner, repo string) ([]string, error)

	// SetVariable creates or updates an environment variable (plaintext, non-secret).
	SetVariable(ctx context.Context, owner, repo, envName, key, value string) error

	// GetVariables returns all variables for a GitHub Environment.
	GetVariables(ctx context.Context, owner, repo, envName string) (map[string]string, error)

	// SetSecret creates or updates an environment secret (encrypted via NaCl sealed box).
	SetSecret(ctx context.Context, owner, repo, envName, key, value string) error

	// DeleteEnvironment deletes a GitHub Environment.
	DeleteEnvironment(ctx context.Context, owner, repo, envName string) error
}

type client struct {
	httpClient  *http.Client
	tokenSource TokenSource
	baseURL     string
}

// NewClient creates a new GitHub Environment client.
func NewClient(tokenSource TokenSource) Client {
	return &client{
		httpClient:  http.DefaultClient,
		tokenSource: tokenSource,
		baseURL:     githubAPIBaseURL,
	}
}

// NewClientWithOptions creates a new GitHub Environment client with custom HTTP client and base URL.
// This is primarily used for testing.
func NewClientWithOptions(tokenSource TokenSource, httpClient *http.Client, baseURL string) Client {
	return &client{
		httpClient:  httpClient,
		tokenSource: tokenSource,
		baseURL:     baseURL,
	}
}

func (c *client) CreateEnvironment(ctx context.Context, owner, repo, envName string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/environments/%s", c.baseURL, owner, repo, envName)
	_, err := c.doRequest(ctx, http.MethodPut, url, nil)
	return err
}

func (c *client) EnvironmentExists(ctx context.Context, owner, repo, envName string) (bool, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/environments/%s", c.baseURL, owner, repo, envName)
	resp, err := c.doRawRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status %d checking environment %q", resp.StatusCode, envName)
}

func (c *client) ListEnvironments(ctx context.Context, owner, repo string) ([]string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/environments", c.baseURL, owner, repo)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Environments []struct {
			Name string `json:"name"`
		} `json:"environments"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse environments response: %w", err)
	}

	names := make([]string, 0, len(result.Environments))
	for _, env := range result.Environments {
		names = append(names, env.Name)
	}
	return names, nil
}

func (c *client) SetVariable(ctx context.Context, owner, repo, envName, key, value string) error {
	repoID, err := c.getRepoID(ctx, owner, repo)
	if err != nil {
		return err
	}

	payload := map[string]string{"name": key, "value": value}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal variable payload: %w", err)
	}

	// Try to create the variable first.
	createURL := fmt.Sprintf("%s/repositories/%d/environments/%s/variables", c.baseURL, repoID, envName)
	resp, err := c.doRawRequest(ctx, http.MethodPost, createURL, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain

	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	// If the variable already exists (409 Conflict), update it.
	if resp.StatusCode == http.StatusConflict {
		updateURL := fmt.Sprintf("%s/repositories/%d/environments/%s/variables/%s", c.baseURL, repoID, envName, key)
		_, updateErr := c.doRequest(ctx, http.MethodPatch, updateURL, body)
		return updateErr
	}

	return fmt.Errorf("failed to set variable %q: unexpected status %d", key, resp.StatusCode)
}

func (c *client) GetVariables(ctx context.Context, owner, repo, envName string) (map[string]string, error) {
	repoID, err := c.getRepoID(ctx, owner, repo)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/repositories/%d/environments/%s/variables", c.baseURL, repoID, envName)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Variables []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse variables response: %w", err)
	}

	vars := make(map[string]string, len(result.Variables))
	for _, v := range result.Variables {
		vars[v.Name] = v.Value
	}
	return vars, nil
}

func (c *client) SetSecret(ctx context.Context, owner, repo, envName, key, value string) error {
	repoID, err := c.getRepoID(ctx, owner, repo)
	if err != nil {
		return err
	}

	// Fetch the environment's public key for secret encryption.
	pkURL := fmt.Sprintf("%s/repositories/%d/environments/%s/secrets/public-key", c.baseURL, repoID, envName)
	pkBody, err := c.doRequest(ctx, http.MethodGet, pkURL, nil)
	if err != nil {
		return fmt.Errorf("failed to get public key for environment %q: %w", envName, err)
	}

	var pk struct {
		KeyID string `json:"key_id"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(pkBody, &pk); err != nil {
		return fmt.Errorf("failed to parse public key response: %w", err)
	}

	encrypted, err := encryptSecret(pk.Key, value)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	payload := map[string]string{
		"encrypted_value": encrypted,
		"key_id":          pk.KeyID,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal secret payload: %w", err)
	}

	secretURL := fmt.Sprintf("%s/repositories/%d/environments/%s/secrets/%s", c.baseURL, repoID, envName, key)
	_, err = c.doRequest(ctx, http.MethodPut, secretURL, payloadBytes)
	return err
}

func (c *client) DeleteEnvironment(ctx context.Context, owner, repo, envName string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/environments/%s", c.baseURL, owner, repo, envName)
	resp, err := c.doRawRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK {
		return nil
	}
	return fmt.Errorf("failed to delete environment %q: unexpected status %d", envName, resp.StatusCode)
}

// getRepoID retrieves the numeric repository ID required by the environment variables API.
func (c *client) getRepoID(ctx context.Context, owner, repo string) (int64, error) {
	url := fmt.Sprintf("%s/repos/%s/%s", c.baseURL, owner, repo)
	body, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to get repository ID for %s/%s: %w", owner, repo, err)
	}

	var result struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, fmt.Errorf("failed to parse repository response: %w", err)
	}
	return result.ID, nil
}

// doRequest performs an authenticated HTTP request and returns the response body.
// It returns an error for non-2xx status codes.
func (c *client) doRequest(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	resp, err := c.doRawRequest(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API error: %s %s returned %d: %s",
			method, url, resp.StatusCode, truncate(string(respBody), 200))
	}
	return respBody, nil
}

// doRawRequest performs an authenticated HTTP request and returns the raw response.
// The caller is responsible for closing the response body.
func (c *client) doRawRequest(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	token, err := c.tokenSource.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// encryptSecret encrypts a secret value using NaCl sealed box with the
// repository's public key, as required by the GitHub Secrets API.
func encryptSecret(publicKeyB64, secretValue string) (string, error) {
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode public key: %w", err)
	}

	if len(publicKeyBytes) != 32 {
		return "", fmt.Errorf("public key must be 32 bytes, got %d", len(publicKeyBytes))
	}

	var recipientKey [32]byte
	copy(recipientKey[:], publicKeyBytes)

	encrypted, err := box.SealAnonymous(nil, []byte(secretValue), &recipientKey, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to seal secret: %w", err)
	}

	return base64.StdEncoding.EncodeToString(encrypted), nil
}

// truncate returns the first n characters of s, appending "..." if truncated.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// StaticTokenSource returns a TokenSource that always returns the same token.
// Useful for testing or when a token is already available.
func StaticTokenSource(token string) TokenSource {
	return &staticTokenSource{token: token}
}

type staticTokenSource struct {
	token string
}

func (s *staticTokenSource) Token(_ context.Context) (string, error) {
	return s.token, nil
}

// RepoIDFromString parses a string repository ID. Exported for testing convenience.
func RepoIDFromString(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}
