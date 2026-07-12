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
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote/auth"
)

func TestGHCRPackageClient_Visibility(t *testing.T) {
	for _, test := range []struct {
		name                string
		accountType         string
		visibility          packageVisibility
		expectedPackagePath string
	}{
		{
			name:                "organization private",
			accountType:         "Organization",
			visibility:          packageVisibilityPrivate,
			expectedPackagePath: "/orgs/radius-project/packages/container/nested%2Fstate",
		},
		{
			name:                "organization internal",
			accountType:         "Organization",
			visibility:          packageVisibilityInternal,
			expectedPackagePath: "/orgs/radius-project/packages/container/nested%2Fstate",
		},
		{
			name:                "user public",
			accountType:         "User",
			visibility:          packageVisibilityPublic,
			expectedPackagePath: "/users/radius-project/packages/container/nested%2Fstate",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				requests++
				require.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))
				require.Equal(t, "application/vnd.github+json", request.Header.Get("Accept"))
				require.Equal(t, githubAPIVersion, request.Header.Get("X-GitHub-Api-Version"))

				switch request.URL.EscapedPath() {
				case "/users/radius-project":
					response.Header().Set("Content-Type", "application/json")
					_, _ = response.Write([]byte(`{"type":"` + test.accountType + `"}`))
				case test.expectedPackagePath:
					response.Header().Set("Content-Type", "application/json")
					_, _ = response.Write([]byte(`{"visibility":"` + string(test.visibility) + `"}`))
				default:
					response.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			client, err := newGHCRPackageClient(
				"ghcr.io/radius-project/nested/state",
				server.URL,
				auth.StaticCredential(ghcrRegistry, auth.Credential{Password: "test-token"}),
				server.Client(),
			)
			require.NoError(t, err)

			visibility, err := client.Visibility(context.Background())
			require.NoError(t, err)
			require.Equal(t, test.visibility, visibility)
			require.Equal(t, 2, requests)
		})
	}
}

func TestGHCRPackageClient_VisibilityReturnsNotFound(t *testing.T) {
	server := newGHCRTestServer(t, "Organization", http.StatusNotFound, "")
	client := newTestGHCRPackageClient(t, server)

	_, err := client.Visibility(context.Background())
	require.ErrorIs(t, err, errGHCRPackageNotFound)
}

func TestGHCRPackageClient_VisibilityFailsClosed(t *testing.T) {
	t.Run("credential error", func(t *testing.T) {
		client, err := newGHCRPackageClient(
			"ghcr.io/radius-project/state",
			"https://api.github.test",
			func(context.Context, string) (auth.Credential, error) {
				return auth.EmptyCredential, errors.New("credential unavailable")
			},
			http.DefaultClient,
		)
		require.NoError(t, err)

		_, err = client.Visibility(context.Background())
		require.ErrorContains(t, err, "credential unavailable")
	})

	t.Run("missing token", func(t *testing.T) {
		client, err := newGHCRPackageClient(
			"ghcr.io/radius-project/state",
			"https://api.github.test",
			auth.StaticCredential(ghcrRegistry, auth.EmptyCredential),
			http.DefaultClient,
		)
		require.NoError(t, err)

		_, err = client.Visibility(context.Background())
		require.ErrorContains(t, err, "do not include a token")
	})

	for _, test := range []struct {
		name          string
		accountType   string
		packageStatus int
		packageBody   string
		errorContains string
	}{
		{
			name:          "unsupported owner type",
			accountType:   "Bot",
			packageStatus: http.StatusOK,
			packageBody:   `{"visibility":"private"}`,
			errorContains: "unsupported GitHub account type",
		},
		{
			name:          "package API failure",
			accountType:   "Organization",
			packageStatus: http.StatusForbidden,
			errorContains: "returned Forbidden",
		},
		{
			name:          "malformed package response",
			accountType:   "Organization",
			packageStatus: http.StatusOK,
			packageBody:   `{`,
			errorContains: "failed to get GHCR package metadata",
		},
		{
			name:          "unknown visibility",
			accountType:   "Organization",
			packageStatus: http.StatusOK,
			packageBody:   `{"visibility":"unknown"}`,
			errorContains: "unsupported visibility",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := newGHCRTestServer(t, test.accountType, test.packageStatus, test.packageBody)
			client := newTestGHCRPackageClient(t, server)

			_, err := client.Visibility(context.Background())
			require.ErrorContains(t, err, test.errorContains)
		})
	}
}

func TestGHCRPackageClient_VisibilityReturnsOwnerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	client := newTestGHCRPackageClient(t, server)

	_, err := client.Visibility(context.Background())
	require.ErrorContains(t, err, "returned Service Unavailable")
}

func TestNewGHCRPackageClientRejectsInvalidRepositories(t *testing.T) {
	for _, repository := range []string{
		"example.test/radius-project/state",
		"ghcr.io/state",
	} {
		t.Run(repository, func(t *testing.T) {
			_, err := newGHCRPackageClient(repository, githubAPIBaseURL, nil, nil)
			require.Error(t, err)
		})
	}
}

func newGHCRTestServer(t *testing.T, accountType string, packageStatus int, packageBody string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.EscapedPath() {
		case "/users/radius-project":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"type":"` + accountType + `"}`))
		case "/orgs/radius-project/packages/container/state", "/users/radius-project/packages/container/state":
			response.WriteHeader(packageStatus)
			if packageBody != "" {
				_, _ = response.Write([]byte(packageBody))
			}
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestGHCRPackageClient(t *testing.T, server *httptest.Server) *ghcrPackageClient {
	t.Helper()

	client, err := newGHCRPackageClient(
		"ghcr.io/radius-project/state",
		server.URL,
		auth.StaticCredential(ghcrRegistry, auth.Credential{Password: "test-token"}),
		server.Client(),
	)
	require.NoError(t, err)
	return client
}
