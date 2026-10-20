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

package oci

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	ghcrRegistry     = "ghcr.io"
	githubAPIBaseURL = "https://api.github.com"
	githubAPIVersion = "2026-03-10"
)

type packageVisibility string

const (
	packageVisibilityPrivate  packageVisibility = "private"
	packageVisibilityPublic   packageVisibility = "public"
	packageVisibilityInternal packageVisibility = "internal"
)

var errGHCRPackageNotFound = errors.New("GHCR package not found")

type packageVisibilityChecker func(context.Context) (packageVisibility, error)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type ghcrPackageClient struct {
	owner       string
	packageName string
	apiBaseURL  string
	credential  auth.CredentialFunc
	httpClient  httpDoer
}

func visibilityCheckerForRepository(repository string) packageVisibilityChecker {
	reference, err := registry.ParseReference(repository)
	if err != nil || !strings.EqualFold(reference.Registry, ghcrRegistry) {
		return nil
	}

	return func(ctx context.Context) (packageVisibility, error) {
		credentialStore, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
			AllowPlaintextPut: true,
		})
		if err != nil {
			return "", fmt.Errorf("failed to read Docker credentials for GHCR visibility check: %w", err)
		}

		client, err := newGHCRPackageClient(repository, githubAPIBaseURL, credentialStore.Get, retry.DefaultClient)
		if err != nil {
			return "", err
		}
		return client.Visibility(ctx)
	}
}

func newGHCRPackageClient(repository, apiBaseURL string, credential auth.CredentialFunc, httpClient httpDoer) (*ghcrPackageClient, error) {
	reference, err := registry.ParseReference(repository)
	if err != nil {
		return nil, fmt.Errorf("invalid GHCR repository %q: %w", repository, err)
	}
	if !strings.EqualFold(reference.Registry, ghcrRegistry) {
		return nil, fmt.Errorf("repository %q is not hosted on GHCR", repository)
	}

	parts := strings.SplitN(reference.Repository, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("GHCR repository %q must include an owner and package name", repository)
	}

	return &ghcrPackageClient{
		owner:       parts[0],
		packageName: parts[1],
		apiBaseURL:  strings.TrimRight(apiBaseURL, "/"),
		credential:  credential,
		httpClient:  httpClient,
	}, nil
}

func (c *ghcrPackageClient) Visibility(ctx context.Context) (packageVisibility, error) {
	credential, err := c.credential(ctx, ghcrRegistry)
	if err != nil {
		return "", fmt.Errorf("failed to read GHCR credentials: %w", err)
	}
	if credential.Password == "" {
		return "", errors.New("GHCR credentials do not include a token for the GitHub Packages API")
	}

	accountType, err := c.accountType(ctx, credential.Password)
	if err != nil {
		return "", err
	}

	owner := url.PathEscape(c.owner)
	packageName := url.PathEscape(c.packageName)
	var packagePath string
	switch accountType {
	case "Organization":
		packagePath = fmt.Sprintf("/orgs/%s/packages/container/%s", owner, packageName)
	case "User":
		packagePath = fmt.Sprintf("/users/%s/packages/container/%s", owner, packageName)
	default:
		return "", fmt.Errorf("unsupported GitHub account type %q for GHCR owner %q", accountType, c.owner)
	}

	var response struct {
		Visibility packageVisibility `json:"visibility"`
	}
	statusCode, err := c.getJSON(ctx, packagePath, credential.Password, &response)
	if err != nil {
		return "", fmt.Errorf("failed to get GHCR package metadata: %w", err)
	}
	if statusCode == http.StatusNotFound {
		return "", errGHCRPackageNotFound
	}
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub Packages API returned %s while checking package %q", http.StatusText(statusCode), c.packageName)
	}

	switch response.Visibility {
	case packageVisibilityPrivate, packageVisibilityPublic, packageVisibilityInternal:
		return response.Visibility, nil
	default:
		return "", fmt.Errorf("GitHub Packages API returned unsupported visibility %q for package %q", response.Visibility, c.packageName)
	}
}

func (c *ghcrPackageClient) accountType(ctx context.Context, token string) (string, error) {
	var response struct {
		Type string `json:"type"`
	}
	statusCode, err := c.getJSON(ctx, "/users/"+url.PathEscape(c.owner), token, &response)
	if err != nil {
		return "", fmt.Errorf("failed to get GHCR owner metadata: %w", err)
	}
	if statusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s while checking GHCR owner %q", http.StatusText(statusCode), c.owner)
	}
	return response.Type, nil
}

func (c *ghcrPackageClient) getJSON(ctx context.Context, path, token string, result any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+path, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-GitHub-Api-Version", githubAPIVersion)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode, nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return 0, err
	}
	return response.StatusCode, nil
}
