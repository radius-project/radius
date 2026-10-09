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
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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

// httpClientTimeoutErr returns the error a real http.Client produces when its
// Timeout elapses, which wraps context.DeadlineExceeded and is a net.Error timeout.
func httpClientTimeoutErr(t *testing.T) error {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	client := &http.Client{Timeout: 10 * time.Millisecond}
	resp, err := client.Get(srv.URL)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	return err
}

type registryClientFunc func(*http.Request) (*http.Response, error)

func (f registryClientFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

func Test_ReadFromRegistry_StopsWhenCallerDeadlineExpires(t *testing.T) {
	original := registryFetchBackoff
	registryFetchBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(5, goretry.NewConstant(time.Hour))
	}
	t.Cleanup(func() { registryFetchBackoff = original })

	requests := 0
	client := registryClientFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return nil, &url.Error{Op: req.Method, URL: req.URL.String(), Err: &net.DNSError{Err: "i/o timeout", Name: "ghcr.io", IsTimeout: true}}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := ReadFromRegistry(ctx, recipes.EnvironmentDefinition{TemplatePath: "ghcr.io/radius-project/recipes/test:1.0"}, &map[string]any{}, client)
	require.Error(t, err)
	require.Less(t, time.Since(start), 10*time.Second)
	require.Equal(t, 1, requests)
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
		{name: "network i/o timeout", err: &url.Error{Op: "Get", URL: "https://ghcr.io", Err: &net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}}, expected: true},
		{name: "server error", err: &errcode.ErrorResponse{StatusCode: http.StatusBadGateway}, expected: true},
		{name: "too many requests", err: &errcode.ErrorResponse{StatusCode: http.StatusTooManyRequests}, expected: true},
		{name: "unauthorized", err: &errcode.ErrorResponse{StatusCode: http.StatusUnauthorized}, expected: false},
		{name: "not found", err: &errcode.ErrorResponse{StatusCode: http.StatusNotFound}, expected: false},
		{name: "context canceled", err: context.Canceled, expected: false},
		{name: "context canceled wrapped in timeout", err: &url.Error{Op: "Get", URL: "https://ghcr.io", Err: fmt.Errorf("%w: %w", context.Canceled, &net.DNSError{IsTimeout: true})}, expected: false},
		{name: "deadline exceeded", err: fmt.Errorf("get: %w", context.DeadlineExceeded), expected: true},
		{name: "http client timeout", err: httpClientTimeoutErr(t), expected: true},
		{name: "generic error", err: errors.New("failed to decode the layers from manifest"), expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, isTransientRegistryError(tc.err))
		})
	}
}

func Test_parsePath(t *testing.T) {
	const sha = "sha256:1234567890123456789012345678901234567890123456789012345678901234"
	tests := []struct {
		name     string
		path     string
		wantRepo string
		wantRef  string
		wantErr  string
	}{
		{name: "tag", path: "ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure:1.0", wantRepo: "ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure", wantRef: "1.0"},
		{name: "no tag defaults to latest", path: "ghcr.io/radius-project/recipes/test", wantRepo: "ghcr.io/radius-project/recipes/test", wantRef: "latest"},
		{name: "digest", path: "ghcr.io/radius-project/recipes/test@" + sha, wantRepo: "ghcr.io/radius-project/recipes/test", wantRef: sha},
		{name: "digest wins over tag", path: "ghcr.io/radius-project/recipes/test:1.0@" + sha, wantRepo: "ghcr.io/radius-project/recipes/test", wantRef: sha},
		{name: "registry port with tag", path: "localhost:5000/recipes/test:1.0", wantRepo: "localhost:5000/recipes/test", wantRef: "1.0"},
		{name: "registry port with digest", path: "localhost:5000/recipes/test@" + sha, wantRepo: "localhost:5000/recipes/test", wantRef: sha},
		{name: "scheme with digest", path: "https://localhost:5000/recipes/test@" + sha, wantRepo: "localhost:5000/recipes/test", wantRef: sha},
		{name: "invalid host", path: "******example.com/test/bar:v1", wantErr: "invalid reference format"},
		{name: "short digest", path: "ghcr.io/radius-project/recipes/test@sha256:1234", wantErr: "invalid reference format"},
		{name: "unsupported digest algorithm", path: "ghcr.io/radius-project/recipes/test@md5:12345678901234567890123456789012", wantErr: "unsupported digest algorithm"},
		{name: "empty digest", path: "ghcr.io/radius-project/recipes/test@", wantErr: "invalid reference format"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repository, ref, err := parsePath(tc.path)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantRepo, repository)
			require.Equal(t, tc.wantRef, ref)
		})
	}
}

// newMovedTagRegistry serves repository "test" with two recipes. Tag "v1" points to
// the second recipe, as if the tag was moved after the first recipe was published.
// Unlike registrytest.NewFakeRegistryServer, it only serves references that exist,
// so tests can tell which reference was resolved. It serves plain HTTP.
func newMovedTagRegistry(t *testing.T) (server *httptest.Server, first digest.Digest) {
	t.Helper()
	manifests := map[string][]byte{}
	blobs := map[string][]byte{}
	addRecipe := func(name string) digest.Digest {
		layer := []byte(`{"recipe":"` + name + `"}`)
		layerDigest := digest.FromBytes(layer)
		blobs[layerDigest.String()] = layer
		manifest := []byte(`{"layers":[{"digest":"` + layerDigest.String() + `"}]}`)
		manifestDigest := digest.FromBytes(manifest)
		manifests[manifestDigest.String()] = manifest
		return manifestDigest
	}
	first = addRecipe("first")
	tags := map[string]string{"v1": addRecipe("second").String()}

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ref := path.Base(r.URL.Path)
		contentType, content := ocispec.MediaTypeImageLayer, blobs
		switch path.Dir(r.URL.Path) {
		case "/v2/test/manifests":
			contentType, content = ocispec.MediaTypeImageManifest, manifests
			if tagged, ok := tags[ref]; ok {
				ref = tagged
			}
		case "/v2/test/blobs":
		default:
			content = nil
		}
		body, ok := content[ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Docker-Content-Digest", ref)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}))
	t.Cleanup(server.Close)
	return server, first
}

func Test_ReadFromRegistry_References(t *testing.T) {
	setFastRegistryBackoff(t, 0)
	server, first := newMovedTagRegistry(t)

	tests := []struct {
		name       string
		path       string
		wantRecipe string
		wantErr    string
	}{
		{name: "tag", path: "/test:v1", wantRecipe: "second"},
		{name: "digest", path: "/test@" + first.String(), wantRecipe: "first"},
		{name: "digest wins over moved tag", path: "/test:v1@" + first.String(), wantRecipe: "first"},
		{name: "missing digest", path: "/test@" + digest.FromString("missing").String(), wantErr: "not found"},
		{name: "missing digest does not fall back to tag", path: "/test:v1@" + digest.FromString("missing").String(), wantErr: "not found"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := map[string]any{}
			err := ReadFromRegistry(t.Context(), recipes.EnvironmentDefinition{TemplatePath: server.URL + tc.path, PlainHTTP: true}, &data, server.Client())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Empty(t, data)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantRecipe, data["recipe"])
		})
	}
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
