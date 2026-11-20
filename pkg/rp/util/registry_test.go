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

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/rp/util/registrytest"
	goretry "github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/errdef"
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

const testDigest = "sha256:1234567890123456789012345678901234567890123456789012345678901234"

// Test_parseTemplatePath checks the normalized reference and RegistryName for valid paths.
// wantRegistry is the key for registry credentials. wantHost is where ORAS sends requests.
// They differ only for Docker Hub.
func Test_parseTemplatePath(t *testing.T) {
	t.Parallel()
	const sha = testDigest
	tests := []struct {
		name           string
		path           string
		wantRegistry   string
		wantRepository string
		wantRef        string
		wantHost       string
	}{
		{name: "tag", path: "ghcr.io/radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure:1.0", wantRegistry: "ghcr.io", wantRepository: "radius-project/dev/recipes/functionaltest/parameters/mongodatabases/azure", wantRef: "1.0", wantHost: "ghcr.io"},
		{name: "no tag defaults to latest", path: "ghcr.io/radius-project/recipes/test", wantRegistry: "ghcr.io", wantRepository: "radius-project/recipes/test", wantRef: "latest", wantHost: "ghcr.io"},
		{name: "digest", path: "ghcr.io/radius-project/recipes/test@" + sha, wantRegistry: "ghcr.io", wantRepository: "radius-project/recipes/test", wantRef: sha, wantHost: "ghcr.io"},
		{name: "digest wins over tag", path: "ghcr.io/radius-project/recipes/test:1.0@" + sha, wantRegistry: "ghcr.io", wantRepository: "radius-project/recipes/test", wantRef: sha, wantHost: "ghcr.io"},

		// Registry ports stay part of the registry.
		{name: "registry port with tag", path: "localhost:5000/recipes/test:1.0", wantRegistry: "localhost:5000", wantRepository: "recipes/test", wantRef: "1.0", wantHost: "localhost:5000"},
		{name: "registry port with digest", path: "localhost:5000/recipes/test@" + sha, wantRegistry: "localhost:5000", wantRepository: "recipes/test", wantRef: sha, wantHost: "localhost:5000"},
		{name: "IPv6 with port", path: "[::1]:5000/recipes/redis:1.0", wantRegistry: "[::1]:5000", wantRepository: "recipes/redis", wantRef: "1.0", wantHost: "[::1]:5000"},
		{name: "explicit 443 is kept", path: "myreg.azurecr.io:443/recipes/redis:1.0", wantRegistry: "myreg.azurecr.io:443", wantRepository: "recipes/redis", wantRef: "1.0", wantHost: "myreg.azurecr.io:443"},

		// A leading lowercase scheme is ignored.
		{name: "https scheme with digest", path: "https://localhost:5000/recipes/test@" + sha, wantRegistry: "localhost:5000", wantRepository: "recipes/test", wantRef: sha, wantHost: "localhost:5000"},
		{name: "http scheme with port", path: "http://registry.example.com:5000/recipes/redis:1.0", wantRegistry: "registry.example.com:5000", wantRepository: "recipes/redis", wantRef: "1.0", wantHost: "registry.example.com:5000"},
		{name: "https scheme on Docker Hub", path: "https://docker.io/myuser/redis:1.0", wantRegistry: "docker.io", wantRepository: "myuser/redis", wantRef: "1.0", wantHost: "registry-1.docker.io"},

		// Docker Hub short names and hosts normalize to docker.io.
		{name: "official image", path: "redis", wantRegistry: "docker.io", wantRepository: "library/redis", wantRef: "latest", wantHost: "registry-1.docker.io"},
		{name: "official image with tag", path: "redis:1.0", wantRegistry: "docker.io", wantRepository: "library/redis", wantRef: "1.0", wantHost: "registry-1.docker.io"},
		{name: "official image with digest", path: "redis@" + sha, wantRegistry: "docker.io", wantRepository: "library/redis", wantRef: sha, wantHost: "registry-1.docker.io"},
		{name: "namespace short name", path: "myuser/redis:1.0", wantRegistry: "docker.io", wantRepository: "myuser/redis", wantRef: "1.0", wantHost: "registry-1.docker.io"},
		{name: "namespace short name with tag and digest", path: "myuser/redis:1.0@" + sha, wantRegistry: "docker.io", wantRepository: "myuser/redis", wantRef: sha, wantHost: "registry-1.docker.io"},
		{name: "index.docker.io", path: "index.docker.io/myuser/redis:1.0", wantRegistry: "docker.io", wantRepository: "myuser/redis", wantRef: "1.0", wantHost: "registry-1.docker.io"},
		{name: "registry-1.docker.io is not rewritten", path: "registry-1.docker.io/myuser/redis:1.0", wantRegistry: "registry-1.docker.io", wantRepository: "myuser/redis", wantRef: "1.0", wantHost: "registry-1.docker.io"},
		{name: "name without dot or port is a Docker Hub namespace", path: "registry/recipes/redis:1.0", wantRegistry: "docker.io", wantRepository: "registry/recipes/redis", wantRef: "1.0", wantHost: "registry-1.docker.io"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ref, err := parseTemplatePath(tc.path)
			require.NoError(t, err)
			require.Equal(t, tc.wantRegistry, ref.Registry)
			require.Equal(t, tc.wantRepository, ref.Repository)
			require.Equal(t, tc.wantRef, ref.ReferenceOrDefault())
			require.Equal(t, tc.wantHost, ref.Host())

			name, err := RegistryName(tc.path)
			require.NoError(t, err)
			require.Equal(t, tc.wantRegistry, name)
		})
	}
}

// Test_parseTemplatePath_Invalid checks that parseTemplatePath and RegistryName reject paths
// that are not valid references. wantErr is the sentinel error that the parser wraps. It is
// nil for an uppercase repository, because distribution/reference wraps no sentinel for it.
func Test_parseTemplatePath_Invalid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{name: "invalid host", path: "******example.com/test/bar:v1", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "space in host", path: "test.azu recr.io/test-private-registry:latest", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "short digest", path: "ghcr.io/radius-project/recipes/test@sha256:1234", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "unsupported digest algorithm", path: "ghcr.io/radius-project/recipes/test@md5:12345678901234567890123456789012", wantErr: digest.ErrDigestUnsupported},
		{name: "empty digest", path: "ghcr.io/radius-project/recipes/test@", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "uppercase scheme", path: "HTTPS://myreg.azurecr.io/recipes/redis:1.0", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "oci scheme", path: "oci://myreg.azurecr.io/recipes/redis:1.0", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "bicep br: syntax", path: "br:myreg.azurecr.io/recipes/redis:1.0", wantErr: reference.ErrReferenceInvalidFormat},
		{name: "uppercase repository", path: "myreg.azurecr.io/Recipes/redis:1.0"},
		{name: "bracketed host that is not an IP address", path: "[000]:0000/000000", wantErr: errdef.ErrInvalidReference},
		{name: "empty", path: "", wantErr: reference.ErrReferenceInvalidFormat},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseTemplatePath(tc.path)
			require.Error(t, err)
			name, nameErr := RegistryName(tc.path)
			require.Error(t, nameErr)
			require.Empty(t, name)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.ErrorIs(t, nameErr, tc.wantErr)
			}
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
	t.Parallel()
	const (
		acrSecretID = "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/acr"
		ecrSecretID = "/planes/radius/local/resourcegroups/default/providers/Applications.Core/secretStores/ecr"
	)
	basicSecret := recipes.SecretData{
		Type: "basicAuthentication",
		Data: map[string]string{
			"username": "test-username",
			"password": "test-password",
		},
	}
	secrets := map[string]recipes.SecretData{acrSecretID: basicSecret}
	acrAndECR := recipes.Configuration{
		RecipeConfig: datamodel.RecipeConfigProperties{
			Bicep: datamodel.BicepConfigProperties{
				Authentication: map[string]datamodel.RegistrySecretConfig{
					"test.azurecr.io": {Secret: acrSecretID},
					"123456789012.dkr.ecr.us-west-2.amazonaws.com": {Secret: ecrSecretID},
				},
			},
		},
	}
	keyed := func(key string, config datamodel.RegistrySecretConfig) recipes.Configuration {
		return recipes.Configuration{
			RecipeConfig: datamodel.RecipeConfigProperties{
				Bicep: datamodel.BicepConfigProperties{
					Authentication: map[string]datamodel.RegistrySecretConfig{key: config},
				},
			},
		}
	}

	testset := []struct {
		name         string
		definition   recipes.Configuration
		templatePath string
		secrets      map[string]recipes.SecretData
		exp          recipes.SecretData
	}{
		{
			name:         "matching registry",
			definition:   acrAndECR,
			templatePath: "test.azurecr.io/test-private-registry:latest",
			secrets:      secrets,
			exp:          basicSecret,
		},
		{
			name:         "no authentication configured",
			definition:   recipes.Configuration{RecipeConfig: datamodel.RecipeConfigProperties{Bicep: datamodel.BicepConfigProperties{}}},
			templatePath: "test.azurecr.io/test-private-registry:latest",
			secrets:      secrets,
			exp:          recipes.SecretData{},
		},
		{
			name:         "docker.io key matches a Docker Hub short name",
			definition:   keyed("docker.io", datamodel.RegistrySecretConfig{Secret: acrSecretID}),
			templatePath: "myuser/redis:1.0",
			secrets:      secrets,
			exp:          basicSecret,
		},
		{
			name:         "key with port matches an http:// path",
			definition:   keyed("registry.example.com:5000", datamodel.RegistrySecretConfig{Secret: acrSecretID}),
			templatePath: "http://registry.example.com:5000/recipes/redis:1.0",
			secrets:      secrets,
			exp:          basicSecret,
		},
		{
			name:         "key matches an https:// path with a digest",
			definition:   keyed("myreg.azurecr.io", datamodel.RegistrySecretConfig{Secret: acrSecretID}),
			templatePath: "https://myreg.azurecr.io/recipes/redis@" + testDigest,
			secrets:      secrets,
			exp:          basicSecret,
		},
		{
			name:         "Docker Hub namespace key does not match",
			definition:   keyed("myuser", datamodel.RegistrySecretConfig{Secret: acrSecretID}),
			templatePath: "myuser/redis:1.0",
			secrets:      secrets,
			exp:          recipes.SecretData{},
		},
		{
			name:         "bicepSettings authentication method selects the type",
			definition:   keyed("docker.io", datamodel.RegistrySecretConfig{Secret: acrSecretID, AuthenticationMethod: "BasicAuth"}),
			templatePath: "myuser/redis:1.0",
			secrets: map[string]recipes.SecretData{
				acrSecretID: {Data: basicSecret.Data},
			},
			exp: basicSecret,
		},
		{
			name:         "bicep br: syntax returns no secrets",
			definition:   keyed("myreg.azurecr.io", datamodel.RegistrySecretConfig{Secret: acrSecretID}),
			templatePath: "br:myreg.azurecr.io/recipes/redis:1.0",
			secrets:      secrets,
			exp:          recipes.SecretData{},
		},
		{
			name:         "space in host returns no secrets",
			definition:   acrAndECR,
			templatePath: "test.azu recr.io/test-private-registry:latest",
			secrets:      secrets,
			exp:          recipes.SecretData{},
		},
	}
	for _, tc := range testset {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			secrets := GetRegistrySecrets(tc.definition, tc.templatePath, tc.secrets)
			require.Equal(t, tc.exp, secrets)
		})
	}
}
