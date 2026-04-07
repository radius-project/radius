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

package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	graphBaseURL    = "https://graph.microsoft.com/v1.0"
	gitHubOIDCIssuer = "https://token.actions.githubusercontent.com"
	azureAudience    = "api://AzureADTokenExchange"
)

// FederatedCredentialConfig describes the federated identity credential to create.
type FederatedCredentialConfig struct {
	// ApplicationObjectID is the Azure AD Application's Object ID (not the Client/App ID).
	ApplicationObjectID string

	// Owner is the GitHub repository owner.
	Owner string

	// Repo is the GitHub repository name.
	Repo string

	// EnvironmentName is the GitHub Environment name used as the subject filter.
	EnvironmentName string
}

// FederationClient creates federated identity credentials on Azure AD applications
// via the Microsoft Graph API.
type FederationClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewFederationClient creates a new FederationClient.
func NewFederationClient() *FederationClient {
	return &FederationClient{
		httpClient: http.DefaultClient,
		baseURL:    graphBaseURL,
	}
}

// NewFederationClientWithOptions creates a FederationClient with custom HTTP client and base URL (for testing).
func NewFederationClientWithOptions(httpClient *http.Client, baseURL string) *FederationClient {
	return &FederationClient{
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// EnsureFederatedCredential creates a federated identity credential on the Azure AD
// application for the given GitHub environment. If a matching credential already exists,
// it returns nil without error.
func (c *FederationClient) EnsureFederatedCredential(ctx context.Context, accessToken string, config FederatedCredentialConfig) error {
	subject := fmt.Sprintf("repo:%s/%s:environment:%s", config.Owner, config.Repo, config.EnvironmentName)
	credName := fmt.Sprintf("radius-%s-%s", config.Repo, config.EnvironmentName)

	// Check if a credential with this subject already exists.
	exists, err := c.federatedCredentialExists(ctx, accessToken, config.ApplicationObjectID, subject)
	if err != nil {
		return fmt.Errorf("failed to check existing federated credentials: %w", err)
	}
	if exists {
		return nil
	}

	// Create the federated identity credential.
	url := fmt.Sprintf("%s/applications/%s/federatedIdentityCredentials", c.baseURL, config.ApplicationObjectID)

	payload := map[string]interface{}{
		"name":        credName,
		"issuer":      gitHubOIDCIssuer,
		"subject":     subject,
		"description": fmt.Sprintf("Radius: GitHub Actions OIDC for %s/%s environment %s", config.Owner, config.Repo, config.EnvironmentName),
		"audiences":   []string{azureAudience},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal federated credential payload: %w", err)
	}

	_, err = c.doRequest(ctx, http.MethodPost, url, accessToken, body)
	if err != nil {
		return fmt.Errorf("failed to create federated identity credential: %w", err)
	}

	return nil
}

// LookupApplicationObjectID finds the Object ID of an Azure AD application by its Client ID (App ID).
func (c *FederationClient) LookupApplicationObjectID(ctx context.Context, accessToken, clientID string) (string, error) {
	url := fmt.Sprintf("%s/applications?$filter=appId eq '%s'&$select=id", c.baseURL, clientID)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, accessToken, nil)
	if err != nil {
		return "", fmt.Errorf("failed to lookup application: %w", err)
	}

	var result struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse application lookup response: %w", err)
	}

	if len(result.Value) == 0 {
		return "", fmt.Errorf("no Azure AD application found with client ID %q", clientID)
	}

	return result.Value[0].ID, nil
}

func (c *FederationClient) federatedCredentialExists(ctx context.Context, accessToken, objectID, subject string) (bool, error) {
	url := fmt.Sprintf("%s/applications/%s/federatedIdentityCredentials", c.baseURL, objectID)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, accessToken, nil)
	if err != nil {
		return false, err
	}

	var result struct {
		Value []struct {
			Subject string `json:"subject"`
		} `json:"value"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, fmt.Errorf("failed to parse federated credentials: %w", err)
	}

	for _, cred := range result.Value {
		if cred.Subject == subject {
			return true, nil
		}
	}

	return false, nil
}

func (c *FederationClient) doRequest(ctx context.Context, method, url, accessToken string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Graph API error: %s %s returned %d: %s",
			method, url, resp.StatusCode, truncate(string(respBody), 300))
	}

	return respBody, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
