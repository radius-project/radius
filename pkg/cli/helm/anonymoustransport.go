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

package helm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

var _ http.RoundTripper = (*anonymousTransport)(nil)

// anonymousTransport implements a http.RoundTripper that override the Authorization token
// when oras client uses the local docker credentials to enforce anonymous pull.
// Note: This is a workaround to enforce anonymous pull for helm chart downloader.
type anonymousTransport struct {
	client      http.Client
	cachedToken string
}

func (t *anonymousTransport) fetchToken(ctx context.Context, realm, service string, scopes []string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm, nil)
	if err != nil {
		return "", err
	}
	q := req.URL.Query()
	if service != "" {
		q.Set("service", service)
	}
	for _, scope := range scopes {
		q.Add("scope", scope)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s %q: failed to get the token with %d", resp.Request.Method, resp.Request.URL, resp.StatusCode)
	}

	// As specified in https://docs.docker.com/registry/spec/auth/token/ section
	// "Token Response Fields", the token is either in `token` or
	// `access_token`. If both present, they are identical.
	var result struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%s %q: failed to decode response: %w", resp.Request.Method, resp.Request.URL, err)
	}
	if result.AccessToken != "" {
		return result.AccessToken, nil
	}
	if result.Token != "" {
		return result.Token, nil
	}

	return "", fmt.Errorf("%s %q: empty token returned", resp.Request.Method, resp.Request.URL)
}

// parseChallenge parses Www-Authenticate header value and returns realm, service and scope.
// https://distribution.github.io/distribution/spec/auth/token/#how-to-authenticate
func parseChallenge(challenge string) (realm, service, scope string) {
	challenge = strings.TrimPrefix(challenge, "Bearer ")
	parts := strings.Split(challenge, ",")
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			switch kv[0] {
			case "realm":
				realm = strings.Trim(kv[1], `"`)
			case "service":
				service = strings.Trim(kv[1], `"`)
			case "scope":
				scope = strings.Trim(kv[1], `"`)
			}
		}
	}
	return
}

func setBearerToken(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
}

func (t *anonymousTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// helm downloader uses the local docker credentials. If it uses the stale credentails,
	// OCI registry returns 401 and invalid Www-Authenticate header.
	// If cached token is available, we override the Authorization header with the cached token.
	if req.Header.Get("Authorization") != "" && t.cachedToken != "" {
		setBearerToken(req, t.cachedToken)
	} else {
		req.Header.Del("Authorization")
	}

	// This is the first try to call the registry.
	resp, err := http.DefaultTransport.RoundTrip(req)

	// If the request is unauthorized, we need to fetch the token and retry.
	// See https://distribution.github.io/distribution/spec/auth/token/#how-to-authenticate
	if err == nil && resp.StatusCode == http.StatusUnauthorized {
		auth := resp.Header.Get("Www-Authenticate")
		if auth != "" {
			realm, service, scope := parseChallenge(auth)
			token, err := t.fetchToken(req.Context(), realm, service, []string{scope})
			if err != nil {
				return nil, err
			}
			t.cachedToken = token
			setBearerToken(req, t.cachedToken)

			// Retry with the token with new token.
			return http.DefaultTransport.RoundTrip(req)
		}
	}

	return resp, err
}

// newAnonymousHTTPClient creates a new http.Client to enforce anonymous pull from the registry.
func newAnonymousHTTPClient() *http.Client {
	return &http.Client{
		Transport: &anonymousTransport{
			client: http.Client{Timeout: time.Duration(10) * time.Second},
		},
	}
}
