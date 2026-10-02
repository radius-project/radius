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

package util

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/rp/util/registrytest"
	goretry "github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func connResetErr() error {
	return &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// flakyRegistryClient wraps a registry client and injects connection resets,
// either before a request reaches the server or while a blob body is read.
type flakyRegistryClient struct {
	client      remote.Client
	resetSends  int
	resetBodies int
	requests    int
}

func (c *flakyRegistryClient) Do(req *http.Request) (*http.Response, error) {
	c.requests++
	if c.resetSends > 0 {
		c.resetSends--
		return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: connResetErr()}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if c.resetBodies > 0 && req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/blobs/") {
		c.resetBodies--
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(errReader{err: connResetErr()})
	}

	return resp, nil
}

func setFastRegistryBackoff(t *testing.T, maxRetries uint64) {
	original := registryFetchBackoff
	registryFetchBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(maxRetries, goretry.NewConstant(time.Millisecond))
	}
	t.Cleanup(func() { registryFetchBackoff = original })
}

func Test_ReadFromRegistry_RetriesTransientFailures(t *testing.T) {
	tests := []struct {
		name        string
		resetSends  int
		resetBodies int
	}{
		{name: "connection reset sending request", resetSends: 2},
		{name: "connection reset reading blob body", resetBodies: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setFastRegistryBackoff(t, 5)
			ts := registrytest.NewFakeRegistryServer(t)
			t.Cleanup(ts.CloseServer)

			client := &flakyRegistryClient{client: ts.TestServer.Client(), resetSends: tc.resetSends, resetBodies: tc.resetBodies}
			data := map[string]any{}
			err := ReadFromRegistry(t.Context(), recipes.EnvironmentDefinition{TemplatePath: ts.TestImageURL}, &data, client)
			require.NoError(t, err)
			require.Contains(t, data, "parameters")
			require.Zero(t, client.resetSends)
			require.Zero(t, client.resetBodies)
		})
	}
}

func Test_ReadFromRegistry_GivesUpAfterRetries(t *testing.T) {
	setFastRegistryBackoff(t, 2)
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)

	client := &flakyRegistryClient{client: ts.TestServer.Client(), resetSends: 100}
	data := map[string]any{}
	err := ReadFromRegistry(t.Context(), recipes.EnvironmentDefinition{TemplatePath: ts.TestImageURL}, &data, client)
	require.ErrorContains(t, err, "connection reset by peer")
	require.Equal(t, 3, client.requests)
}

func Test_ReadFromRegistry_DoesNotRetryNotFound(t *testing.T) {
	ts := registrytest.NewFakeRegistryServer(t)
	t.Cleanup(ts.CloseServer)
	definition := recipes.EnvironmentDefinition{TemplatePath: ts.TestServer.URL + "/nonexisting:latest"}

	setFastRegistryBackoff(t, 0)
	single := &flakyRegistryClient{client: ts.TestServer.Client()}
	err := ReadFromRegistry(t.Context(), definition, &map[string]any{}, single)
	require.ErrorContains(t, err, "not found")

	setFastRegistryBackoff(t, 5)
	retrying := &flakyRegistryClient{client: ts.TestServer.Client()}
	err = ReadFromRegistry(t.Context(), definition, &map[string]any{}, retrying)
	require.ErrorContains(t, err, "not found")
	require.Equal(t, single.requests, retrying.requests)
}

func Test_isTransientRegistryError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{name: "connection reset", err: &url.Error{Op: "Get", URL: "https://ghcr.io", Err: connResetErr()}, expected: true},
		{name: "connection refused", err: fmt.Errorf("wrapped: %w", syscall.ECONNREFUSED), expected: true},
		{name: "unexpected EOF", err: fmt.Errorf("read: %w", io.ErrUnexpectedEOF), expected: true},
		{name: "dns timeout", err: &url.Error{Op: "Head", URL: "https://ghcr.io", Err: &net.DNSError{Err: "i/o timeout", Name: "ghcr.io", IsTimeout: true}}, expected: true},
		{name: "dns not found", err: &net.DNSError{Err: "no such host", Name: "ghcr.io", IsNotFound: true}, expected: false},
		{name: "server error", err: &errcode.ErrorResponse{StatusCode: http.StatusBadGateway}, expected: true},
		{name: "too many requests", err: &errcode.ErrorResponse{StatusCode: http.StatusTooManyRequests}, expected: true},
		{name: "unauthorized", err: &errcode.ErrorResponse{StatusCode: http.StatusUnauthorized}, expected: false},
		{name: "not found", err: &errcode.ErrorResponse{StatusCode: http.StatusNotFound}, expected: false},
		{name: "context canceled", err: context.Canceled, expected: false},
		{name: "deadline exceeded", err: fmt.Errorf("get: %w", context.DeadlineExceeded), expected: false},
		{name: "generic error", err: errors.New("failed to decode the layers from manifest"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, isTransientRegistryError(tc.err))
		})
	}
}

func Test_PathParser(t *testing.T) {
	repository, tag, err := parsePath("ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure:1.0")
	require.NoError(t, err)
	require.Equal(t, "ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure", repository)
	require.Equal(t, "1.0", tag)
}

func Test_PathParserErr(t *testing.T) {
	repository, tag, err := parsePath("http://user:passwd@example.com/test/bar:v1")
	require.Error(t, err)
	require.Equal(t, "", repository)
	require.Equal(t, "", tag)
}

func Test_PathParserDigestErr(t *testing.T) {
	// A digest reference carries no tag, so it must be rejected at parse time
	// rather than failing later when the registry is resolved by tag.
	repository, tag, err := parsePath("ghcr.io/radius-project/recipes/test@sha256:1234567890123456789012345678901234567890123456789012345678901234")
	require.Error(t, err)
	require.Contains(t, err.Error(), "a tagged reference such as repository:tag is required")
	require.Equal(t, "", repository)
	require.Equal(t, "", tag)
}

func Test_GetRegistrySecrets(t *testing.T) {
	testset := []struct {
		definition   recipes.Configuration
		templatePath string
		secrets      map[string]recipes.SecretData
		exp          recipes.SecretData
		err          string
	}{
		{
			definition: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Bicep: datamodel.BicepConfigProperties{
						Authentication: map[string]datamodel.RegistrySecretConfig{
							"test.azurecr.io": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr",
							},
							"123456789012.dkr.ecr.us-west-2.amazonaws.com": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/ecr",
							},
						},
					},
				},
			},
			templatePath: "test.azurecr.io/test-private-registry:latest",
			secrets: map[string]recipes.SecretData{
				"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr": {
					Type: "basicAuthentication",
					Data: map[string]string{
						"username": "test-username",
						"password": "test-password",
					},
				},
			},
			exp: recipes.SecretData{
				Type: "basicAuthentication",
				Data: map[string]string{
					"username": "test-username",
					"password": "test-password",
				},
			},
		},
		{
			definition: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Bicep: datamodel.BicepConfigProperties{},
				},
			},
			templatePath: "test.azurecr.io/test-private-registry:latest",
			secrets: map[string]recipes.SecretData{
				"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr": {
					Type: "basicAuthentication",
					Data: map[string]string{
						"username": "test-username",
						"password": "test-password",
					},
				},
			},
			exp: recipes.SecretData{},
		},
		{
			definition: recipes.Configuration{
				RecipeConfig: datamodel.RecipeConfigProperties{
					Bicep: datamodel.BicepConfigProperties{
						Authentication: map[string]datamodel.RegistrySecretConfig{
							"test.azurecr.io": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr",
							},
							"123456789012.dkr.ecr.us-west-2.amazonaws.com": {
								Secret: "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/ecr",
							},
						},
					},
				},
			},
			templatePath: "test.azu recr.io/test-private-registry:latest",
			secrets: map[string]recipes.SecretData{
				"/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr": {
					Type: "basicAuthentication",
					Data: map[string]string{
						"username": "test-username",
						"password": "test-password",
					},
				},
			},
			exp: recipes.SecretData{},
			err: "invalid character \" \" in host name",
		},
	}
	for _, tc := range testset {
		secrets, err := GetRegistrySecrets(tc.definition, tc.templatePath, tc.secrets)
		if tc.err != "" {
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.err)
		} else {
			require.NoError(t, err)
			require.Equal(t, secrets, tc.exp)
		}
	}
}
