/*
Copyright 2025 The Radius Authors.

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
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	goretry "github.com/sethvargo/go-retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/registry"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	"oras.land/oras-go/v2/registry/remote/errcode"
)

func Test_isTransientHelmChartError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil", nil, false},
		{"HTTP 500", errors.New("failed to fetch https://example.com/contour.tgz : 500 Internal Server Error"), true},
		{"wrapped HTTP 503", fmt.Errorf("index: %w", errors.New("failed to fetch https://example.com/index.yaml : 503 Service Unavailable")), true},
		{"HTTP 408", errors.New("failed to fetch http://example.com/chart.tgz : 408 Request Timeout"), true},
		{"HTTP 429", errors.New("failed to fetch https://example.com/chart.tgz : 429 Too Many Requests"), true},
		{"HTTP 502", errors.New("failed to fetch https://example.com/chart.tgz : 502 Bad Gateway"), true},
		{"HTTP 504", errors.New("failed to fetch https://example.com/chart.tgz : 504 Gateway Timeout"), true},
		{"HTTP 401", errors.New("failed to fetch https://example.com/chart.tgz : 401 Unauthorized"), false},
		{"HTTP 403", errors.New("failed to fetch https://example.com/chart.tgz : 403 Forbidden"), false},
		{"HTTP 404", errors.New("failed to fetch https://example.com/chart.tgz : 404 Not Found"), false},
		{"HTTP 501", errors.New("failed to fetch https://example.com/chart.tgz : 501 Not Implemented"), false},
		{"status in URL", errors.New("failed to fetch https://example.com/500 : 404 Not Found"), false},
		{"unrelated error", errors.New("chart version 500 not found"), false},
		{"canceled", fmt.Errorf("pull: %w", context.Canceled), false},
		{"permanent DNS", &net.DNSError{IsNotFound: true}, false},
		{"temporary DNS", &net.DNSError{IsTemporary: true}, true},
		{"timeout", &url.Error{Op: "Get", URL: "https://example.com", Err: context.DeadlineExceeded}, true},
		{"reset", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		{"refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), true},
		{"broken pipe", fmt.Errorf("read: %w", syscall.EPIPE), true},
		{"short response", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), true},
		{"closed connection", &url.Error{Op: "Get", URL: "https://example.com", Err: io.EOF}, true},
		{"local file", os.ErrPermission, false},
	}
	for _, status := range []int{408, 429, 500, 502, 503, 504, 401, 403, 404, 501} {
		tests = append(tests, struct {
			name      string
			err       error
			retryable bool
		}{
			fmt.Sprintf("OCI %d", status),
			fmt.Errorf("pull: %w", &errcode.ErrorResponse{StatusCode: status}),
			status == 408 || status == 429 || status == 500 || status == 502 || status == 503 || status == 504,
		})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.retryable, isTransientHelmChartError(tt.err))
		})
	}
}

// Broken user Helm states that chart pulls must ignore. Paths are relative to the root of the
// environment created by setUserHelmEnv.
var (
	// missingRepositoryIndex reproduces https://github.com/radius-project/radius/issues/13546. On
	// Windows, repositories.yaml persists under %APPDATA% while cached indexes under %TEMP% can be
	// cleaned up.
	missingRepositoryIndex = map[string]string{
		"config/repositories.yaml": "repositories:\n- name: kedacore\n  url: https://kedacore.github.io/charts\n",
	}
	corruptRepositoryIndex = map[string]string{
		"config/repositories.yaml":            "repositories:\n- name: bitnami\n  url: https://charts.bitnami.com/bitnami\n",
		"cache/repository/bitnami-index.yaml": "apiVersion: v1\nentries: [truncated",
	}
	corruptRepositoryConfig = map[string]string{
		"config/repositories.yaml": "repositories:\n- name: kedacore\n  url: [truncated",
	}
	// unwritableUserCache replaces the Helm cache directory with a regular file, so that writing to the
	// user's Helm cache fails on every OS, as it does in a read-only home directory or container.
	unwritableUserCache = map[string]string{
		"cache": "not a directory",
	}
)

// userHelmEnv simulates the Helm environment of the user running rad. Chart pulls must neither
// depend on nor modify it.
type userHelmEnv struct {
	root    string
	entries map[string]string
}

// setUserHelmEnv points every Helm state location at a temporary directory seeded with the files of
// the given states, and snapshots it. Tests that use it can't run in parallel.
func setUserHelmEnv(t *testing.T, states ...map[string]string) *userHelmEnv {
	t.Helper()
	root := t.TempDir()
	for name, value := range map[string]string{
		"HELM_CONFIG_HOME":       filepath.Join(root, "config"),
		"HELM_CACHE_HOME":        filepath.Join(root, "cache"),
		"HELM_DATA_HOME":         filepath.Join(root, "data"),
		"HELM_REPOSITORY_CONFIG": filepath.Join(root, "config", "repositories.yaml"),
		"HELM_REPOSITORY_CACHE":  filepath.Join(root, "cache", "repository"),
		"HELM_CONTENT_CACHE":     filepath.Join(root, "cache", "content"),
		"HELM_REGISTRY_CONFIG":   filepath.Join(root, "config", "registry", "config.json"),
		"HELM_PLUGINS":           filepath.Join(root, "data", "plugins"),
		"DOCKER_CONFIG":          filepath.Join(root, "docker"),
	} {
		t.Setenv(name, value)
	}

	files := map[string]string{"config/registry/config.json": `{"auths":{}}`}
	for _, state := range states {
		maps.Copy(files, state)
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	return &userHelmEnv{root: root, entries: snapshot(t, root)}
}

// assertUnchanged asserts that no file or directory in the user's Helm environment was added, removed,
// or modified.
func (e *userHelmEnv) assertUnchanged(t *testing.T) {
	t.Helper()
	assert.Equal(t, e.entries, snapshot(t, e.root), "chart pulls must not change the user's Helm state")
}

// snapshot maps the slash-separated path of each file and directory under root to the file's content.
// Directory paths end with a slash.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	entries := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == root {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			entries[filepath.ToSlash(rel)+"/"] = ""
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		entries[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	require.NoError(t, err)
	return entries
}

// pathWithin reports whether path is parent or is inside it.
func pathWithin(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && filepath.IsLocal(rel)
}

const contourIndexYAML = "apiVersion: v1\nentries:\n  contour:\n  - name: contour\n    version: 0.1.0\n    apiVersion: v2\n    urls:\n    - contour-0.1.0.tgz\n"

// chartArchive returns the archive of an empty chart with the given name and version 0.1.0.
func chartArchive(t *testing.T, name string) []byte {
	t.Helper()
	archive, err := chartutil.Save(&chart.Chart{Metadata: &chart.Metadata{
		APIVersion: "v2",
		Name:       name,
		Version:    "0.1.0",
	}}, t.TempDir())
	require.NoError(t, err)
	content, err := os.ReadFile(archive)
	require.NoError(t, err)
	return content
}

func Test_HelmChartFromContainerRegistry_BrokenUserHelmState(t *testing.T) {
	chartBytes := chartArchive(t, "contour")

	tests := []struct {
		name  string
		files map[string]string
	}{
		{"registered repository index missing", missingRepositoryIndex},
		{"registered repository index corrupt", corruptRepositoryIndex},
		{"repositories file corrupt", corruptRepositoryConfig},
		{"user cache unwritable", unwritableUserCache},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setUserHelmEnv(t, tt.files)
			var archiveRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/index.yaml":
					_, _ = fmt.Fprint(w, contourIndexYAML)
				case "/contour-0.1.0.tgz":
					archiveRequests.Add(1)
					_, _ = w.Write(chartBytes)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			got, err := NewHelmAction(NewHelmClient()).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, server.URL, "contour")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "contour", got.Metadata.Name)
			assert.Equal(t, "0.1.0", got.Metadata.Version)
			assert.EqualValues(t, 1, archiveRequests.Load())
			env.assertUnchanged(t)
		})
	}
}

// Test_HelmChartFromContainerRegistry_PrivatePullState checks the Helm settings that each pull receives,
// so a pull that uses the user's Helm repository state fails here even if none of the broken states
// above triggers a failure.
func Test_HelmChartFromContainerRegistry_PrivatePullState(t *testing.T) {
	env := setUserHelmEnv(t, missingRepositoryIndex)

	tests := []struct {
		name     string
		repoURL  string
		release  string
		chartRef string
	}{
		{"HTTP repository", "https://projectcontour.github.io/helm-charts", "contour", "contour"},
		{"OCI registry", "oci://ghcr.io/radius-project/helm-chart", "radius", "oci://ghcr.io/radius-project/helm-chart/radius"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pullRoot string
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().RunHelmPull(gomock.Any(), tt.chartRef).
				DoAndReturn(func(opts []helm.PullOpt, _ string) (string, error) {
					pull := helm.NewPull(opts...)
					require.NotNil(t, pull.Settings)
					pullRoot = filepath.Dir(pull.DestDir)
					for name, path := range map[string]string{
						"repository config": pull.Settings.RepositoryConfig,
						"repository cache":  pull.Settings.RepositoryCache,
						"content cache":     pull.Settings.ContentCache,
					} {
						require.NotEmpty(t, path, name)
						assert.False(t, pathWithin(env.root, path), "%s must not be the user's: %s", name, path)
						assert.True(t, pathWithin(pullRoot, path), "%s must be private to the pull: %s", name, path)
						assert.False(t, pathWithin(pull.DestDir, path), "%s must be outside the download directory: %s", name, path)
					}
					assert.NoFileExists(t, pull.Settings.RepositoryConfig, "chart pulls must not see any registered repositories")
					return "", os.WriteFile(filepath.Join(pull.DestDir, "chart.tgz"), []byte("chart"), 0o600)
				}).Times(1)
			want := &chart.Chart{}
			client.EXPECT().LoadChart(gomock.Any()).Return(want, nil).Times(1)

			config := &helm.Configuration{}
			got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", config, tt.repoURL, tt.release)
			require.NoError(t, err)
			assert.Same(t, want, got)
			assert.NoDirExists(t, pullRoot, "private pull state must be removed after the pull")
			assert.Nil(t, config.RegistryClient, "a pull must not leave its registry client in the caller's configuration")
			env.assertUnchanged(t)
		})
	}
}

const (
	fakeChartRegistryHost       = "charts.radius.test"
	fakeChartRegistryRepository = "charts/radius"
	fakeChartRegistryUser       = "radius"
	fakeChartRegistryPassword   = "password"
	fakeChartRegistryToken      = "pull-token"
)

type registryContent struct {
	mediaType string
	data      []byte
}

// fakeChartRegistry is an in-process OCI registry that serves a radius 0.1.0 chart from one repository. Like
// ghcr.io, it requires a bearer token, which its token endpoint issues for the user's credentials.
type fakeChartRegistry struct {
	host       string
	repository string
	content    map[string]registryContent

	tokenRequests      atomic.Int32 // requests for a token
	challengedRequests atomic.Int32 // requests without a token
	authorizedRequests atomic.Int32 // requests with a token
	tagListRequests    atomic.Int32 // requests with a token for the tag list
}

// newFakeChartRegistry returns a fake registry for the repository on the host. Install it with
// setDefaultTransport, so that OCI pulls reach it without a network or TLS.
func newFakeChartRegistry(t *testing.T, host, repository string) *fakeChartRegistry {
	t.Helper()
	config := []byte(`{"apiVersion":"v2","name":"radius","version":"0.1.0"}`)
	chartBytes := chartArchive(t, "radius")
	manifest := fmt.Appendf(nil, `{"schemaVersion":2,"mediaType":%q,"config":{"mediaType":%q,"digest":%q,"size":%d},"layers":[{"mediaType":%q,"digest":%q,"size":%d}]}`,
		ocispec.MediaTypeImageManifest,
		registry.ConfigMediaType, digest.FromBytes(config), len(config),
		registry.ChartLayerMediaType, digest.FromBytes(chartBytes), len(chartBytes))
	tags := fmt.Appendf(nil, `{"name":%q,"tags":["0.0.9","0.1.0"]}`, repository)
	prefix := "/v2/" + repository
	return &fakeChartRegistry{host: host, repository: repository, content: map[string]registryContent{
		prefix + "/tags/list":       {"application/json", tags},
		prefix + "/manifests/0.1.0": {ocispec.MediaTypeImageManifest, manifest},
		prefix + "/manifests/" + digest.FromBytes(manifest).String(): {ocispec.MediaTypeImageManifest, manifest},
		prefix + "/blobs/" + digest.FromBytes(config).String():       {registry.ConfigMediaType, config},
		prefix + "/blobs/" + digest.FromBytes(chartBytes).String():   {registry.ChartLayerMediaType, chartBytes},
	}}
}

// setDefaultTransport makes http.DefaultTransport, which Helm's registry client uses, send requests to rt for
// the rest of the test.
func setDefaultTransport(t *testing.T, rt http.RoundTripper) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = original })
}

// registryCredentials returns a user Helm registry configuration with credentials for the registry on the host.
func registryCredentials(host, password string) map[string]string {
	auth := base64.StdEncoding.EncodeToString([]byte(fakeChartRegistryUser + ":" + password))
	return map[string]string{
		"config/registry/config.json": fmt.Sprintf(`{"auths":{%q:{"auth":%q}}}`, host, auth),
	}
}

func (f *fakeChartRegistry) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	f.serve(recorder, req)
	resp := recorder.Result()
	resp.Request = req
	return resp, nil
}

func (f *fakeChartRegistry) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Host != f.host {
		http.NotFound(w, r)
		return
	}

	if r.URL.Path == "/token" {
		f.tokenRequests.Add(1)
		if user, password, ok := r.BasicAuth(); !ok || user != fakeChartRegistryUser || password != fakeChartRegistryPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"token":%q}`, fakeChartRegistryToken)
		return
	}

	if r.Header.Get("Authorization") != "Bearer "+fakeChartRegistryToken {
		f.challengedRequests.Add(1)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="https://%s/token",service=%q,scope="repository:%s:pull"`, f.host, f.host, f.repository))
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.authorizedRequests.Add(1)
	if strings.HasSuffix(r.URL.Path, "/tags/list") {
		f.tagListRequests.Add(1)
	}

	content, ok := f.content[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", content.mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(content.data)))
	w.Header().Set("Docker-Content-Digest", digest.FromBytes(content.data).String())
	if r.Method != http.MethodHead {
		_, _ = w.Write(content.data)
	}
}

// Test_HelmChartFromContainerRegistry_OCICredentials checks that OCI pulls still authenticate with the
// credentials in the user's Helm registry configuration, while ignoring the user's broken Helm state.
func Test_HelmChartFromContainerRegistry_OCICredentials(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"user credentials", fakeChartRegistryPassword, false},
		{"wrong user credentials", "wrong", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setUserHelmEnv(t, missingRepositoryIndex, unwritableUserCache, registryCredentials(fakeChartRegistryHost, tt.password))
			chartRegistry := newFakeChartRegistry(t, fakeChartRegistryHost, fakeChartRegistryRepository)
			setDefaultTransport(t, chartRegistry)

			got, err := NewHelmAction(NewHelmClient()).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, "oci://"+fakeChartRegistryHost+"/charts", "radius")
			if tt.wantErr {
				respErr, ok := errors.AsType[*errcode.ErrorResponse](err)
				require.True(t, ok, "expected a registry error response, got: %v", err)
				assert.Equal(t, http.StatusUnauthorized, respErr.StatusCode)
				assert.Nil(t, got)
				assert.Zero(t, chartRegistry.authorizedRequests.Load())
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "radius", got.Metadata.Name)
				assert.Equal(t, "0.1.0", got.Metadata.Version)
				assert.Positive(t, chartRegistry.authorizedRequests.Load())
			}
			assert.EqualValues(t, 1, chartRegistry.tokenRequests.Load(), "an attempt must fetch a registry token once")
			env.assertUnchanged(t)
		})
	}
}

// Test_HelmChartFromContainerRegistry_OCIVersions checks pulls of each kind of chart version that Radius asks an
// OCI registry for: an exact version, a version range such as the ~X.Y.Z of release builds, and the latest version,
// which GetLatestRadiusVersion asks for. A range or the latest version needs the registry's tag list. Each attempt
// must fetch a registry token once and reuse it for its other requests.
func Test_HelmChartFromContainerRegistry_OCIVersions(t *testing.T) {
	pullChart := func(version string) func(context.Context, HelmClient) (string, error) {
		return func(ctx context.Context, client HelmClient) (string, error) {
			got, err := NewHelmAction(client).HelmChartFromContainerRegistry(ctx, version, &helm.Configuration{}, "oci://"+fakeChartRegistryHost+"/charts", "radius")
			if err != nil {
				return "", err
			}
			return got.Metadata.Version, nil
		}
	}
	latestRadiusVersion := func(ctx context.Context, client HelmClient) (string, error) {
		return (&Impl{Helm: client}).GetLatestRadiusVersion(ctx)
	}
	tests := []struct {
		name        string
		host        string
		repository  string
		pull        func(context.Context, HelmClient) (string, error)
		wantTagList bool
	}{
		{"exact version", fakeChartRegistryHost, fakeChartRegistryRepository, pullChart("0.1.0"), false},
		{"version range", fakeChartRegistryHost, fakeChartRegistryRepository, pullChart("~0.1.0"), true},
		{"latest version", fakeChartRegistryHost, fakeChartRegistryRepository, pullChart(""), true},
		{"latest Radius version", "ghcr.io", "radius-project/helm-chart/radius", latestRadiusVersion, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setUserHelmEnv(t, missingRepositoryIndex, unwritableUserCache, registryCredentials(tt.host, fakeChartRegistryPassword))
			chartRegistry := newFakeChartRegistry(t, tt.host, tt.repository)
			setDefaultTransport(t, chartRegistry)

			version, err := tt.pull(t.Context(), NewHelmClient())
			require.NoError(t, err)
			assert.Equal(t, "0.1.0", version)
			assert.Equal(t, tt.wantTagList, chartRegistry.tagListRequests.Load() > 0, "tag list requests: %d", chartRegistry.tagListRequests.Load())
			assert.EqualValues(t, 1, chartRegistry.tokenRequests.Load(), "an attempt must fetch a registry token once")
			assert.EqualValues(t, 1, chartRegistry.challengedRequests.Load(), "an attempt must reuse its registry token")
			env.assertUnchanged(t)
		})
	}
}

func Test_HelmChartFromContainerRegistry_HTTPRetry(t *testing.T) {
	originalBackoff := helmChartPullBackoff
	helmChartPullBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(2, goretry.NewConstant(time.Millisecond))
	}
	t.Cleanup(func() { helmChartPullBackoff = originalBackoff })
	chartBytes := chartArchive(t, "contour")

	tests := []struct {
		name         string
		failPath     string
		status       int
		failures     int
		wantRequests int
		wantError    bool
	}{
		{"first attempt succeeds", "", 0, 0, 0, false},
		{"chart recovers from 500", "/contour-0.1.0.tgz", 500, 1, 2, false},
		{"index recovers from 503", "/index.yaml", 503, 1, 2, false},
		{"rate limit recovers", "/contour-0.1.0.tgz", 429, 2, 3, false},
		{"chart recovers from EOF", "/contour-0.1.0.tgz", 0, 1, 2, false},
		{"chart recovers from reset", "/contour-0.1.0.tgz", -1, 1, 2, false},
		{"chart retry limit", "/contour-0.1.0.tgz", 500, 10, 3, true},
		{"index retry limit", "/index.yaml", 503, 10, 3, true},
		{"missing chart", "/contour-0.1.0.tgz", 404, 10, 1, true},
		{"index unauthorized", "/index.yaml", 401, 10, 1, true},
		{"chart forbidden", "/contour-0.1.0.tgz", 403, 10, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use broken rather than pristine user Helm state, so pulls that depend on it fail here.
			env := setUserHelmEnv(t, missingRepositoryIndex)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == tt.failPath {
					if requests.Add(1) <= int32(tt.failures) {
						if tt.status <= 0 {
							conn, _, err := http.NewResponseController(w).Hijack()
							if err != nil {
								t.Errorf("hijack connection: %v", err)
								return
							}
							if tt.status == -1 {
								tcpConn, ok := conn.(*net.TCPConn)
								if !ok {
									t.Errorf("expected TCP connection, got %T", conn)
								} else if err := tcpConn.SetLinger(0); err != nil {
									t.Errorf("set reset on close: %v", err)
								}
							}
							if err := conn.Close(); err != nil {
								t.Errorf("close connection: %v", err)
							}
							return
						}
						http.Error(w, http.StatusText(tt.status), tt.status)
						return
					}
				}
				switch r.URL.Path {
				case "/index.yaml":
					_, _ = fmt.Fprint(w, contourIndexYAML)
				case "/contour-0.1.0.tgz":
					_, _ = w.Write(chartBytes)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			action := NewHelmAction(NewHelmClient())
			got, err := action.HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, server.URL, "contour")
			if tt.wantError {
				require.Error(t, err)
				assert.Nil(t, got)
				assert.Contains(t, err.Error(), fmt.Sprintf("%d %s", tt.status, http.StatusText(tt.status)))
				assert.Contains(t, err.Error(), "version: 0.1.0, release name: contour")
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, "contour", got.Metadata.Name)
			}
			assert.EqualValues(t, tt.wantRequests, requests.Load())
			env.assertUnchanged(t)
		})
	}
}

// recordingHelmClient records each attempt to pull a chart.
type recordingHelmClient struct {
	HelmClient
	pulls []recordedPull
}

type recordedPull struct {
	duration time.Duration
	err      error
}

func (c *recordingHelmClient) RunHelmPull(pullopts []helm.PullOpt, chartRef string) (string, error) {
	start := time.Now()
	out, err := c.HelmClient.RunHelmPull(pullopts, chartRef)
	c.pulls = append(c.pulls, recordedPull{duration: time.Since(start), err: err})
	return out, err
}

// slowTransport answers each request after the delay that delay returns for it, unless the request ends first.
type slowTransport struct {
	next  http.RoundTripper
	delay func(*http.Request) time.Duration
}

func (s *slowTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if delay := s.delay(req); delay > 0 {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(delay):
		}
	}
	return s.next.RoundTrip(req)
}

// chartPullLog holds the messages that chart pulls showed the user.
type chartPullLog struct {
	lines []string
}

// recordChartPullLog records the messages that chart pulls show the user for the rest of the test, instead of
// showing them.
func recordChartPullLog(t *testing.T) *chartPullLog {
	t.Helper()
	logged := &chartPullLog{}
	original := logChartPull
	logChartPull = func(format string, args ...any) { logged.lines = append(logged.lines, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { logChartPull = original })
	return logged
}

// requireTimeout requires err to come from a request that timed out.
func requireTimeout(t *testing.T, err error) {
	t.Helper()
	netErr, ok := errors.AsType[net.Error](err)
	require.True(t, ok && netErr.Timeout(), "expected a request timeout, got: %v", err)
	require.True(t, isTransientHelmChartError(err), "a request timeout must be retried: %v", err)
}

// Test_HelmChartFromContainerRegistry_RetriesTimedOutAttempts checks that the production retry window fits three
// attempts that each take the whole attempt timeout and then fail because a request timed out. Before, a request
// could take Helm's two-minute timeout, longer than the whole retry window, so it was never retried.
func Test_HelmChartFromContainerRegistry_RetriesTimedOutAttempts(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		release  string
		chartRef string
	}{
		{"HTTP repository", "https://projectcontour.github.io/helm-charts", "contour", "contour"},
		{"OCI registry", "oci://ghcr.io/radius-project/helm-chart", "radius", "oci://ghcr.io/radius-project/helm-chart/radius"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setUserHelmEnv(t)
			logged := recordChartPullLog(t)
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				client := NewMockHelmClient(gomock.NewController(t))
				client.EXPECT().RunHelmPull(gomock.Any(), tt.chartRef).
					DoAndReturn(func([]helm.PullOpt, string) (string, error) {
						attempts++
						time.Sleep(chartPullAttemptTimeout())
						return "", &url.Error{Op: "Get", URL: "https://example.com/chart.tgz", Err: context.DeadlineExceeded}
					}).AnyTimes()

				start := time.Now()
				got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, tt.repoURL, tt.release)
				assert.Nil(t, got)
				requireTimeout(t, err)
				assert.GreaterOrEqual(t, attempts, 3, "the retry window must fit three attempts that time out")
				assert.LessOrEqual(t, time.Since(start), chartPullRetryWindow+chartPullAttemptTimeout(), "a pull must end within the retry window and one more attempt")
				assert.Len(t, logged.lines, attempts-1, "each retry must be shown")
			})
		})
	}
}

// Test_HelmChartFromContainerRegistry_LogsRetries checks that a pull shows the user each failed attempt that it
// retries, and nothing when it doesn't retry.
func Test_HelmChartFromContainerRegistry_LogsRetries(t *testing.T) {
	originalBackoff := helmChartPullBackoff
	helmChartPullBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(2, goretry.NewConstant(time.Millisecond))
	}
	t.Cleanup(func() { helmChartPullBackoff = originalBackoff })

	unavailable := errors.New("failed to fetch https://example.com/contour-0.1.0.tgz : 503 Service Unavailable")
	notFound := errors.New("failed to fetch https://example.com/contour-0.1.0.tgz : 404 Not Found")
	retrying := func(attempt int) string {
		return fmt.Sprintf("Downloading the contour chart failed (attempt %d), retrying: %v", attempt, unavailable)
	}
	tests := []struct {
		name     string
		attempts []error // the error of each attempt, or nil when it succeeds
		wantLog  []string
	}{
		{"first attempt succeeds", []error{nil}, nil},
		{"first attempt fails without a retry", []error{notFound}, nil},
		{"second attempt succeeds", []error{unavailable, nil}, []string{retrying(1)}},
		{"retries run out", []error{unavailable, unavailable, unavailable}, []string{retrying(1), retrying(2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logged := recordChartPullLog(t)
			client := NewMockHelmClient(gomock.NewController(t))
			var calls []any
			for _, attemptErr := range tt.attempts {
				calls = append(calls, client.EXPECT().RunHelmPull(gomock.Any(), "contour").
					DoAndReturn(func(opts []helm.PullOpt, _ string) (string, error) {
						if attemptErr != nil {
							return "", attemptErr
						}
						return "", os.WriteFile(filepath.Join(helm.NewPull(opts...).DestDir, "chart.tgz"), []byte("chart"), 0o600)
					}))
			}
			gomock.InOrder(calls...)
			succeeds := tt.attempts[len(tt.attempts)-1] == nil
			if succeeds {
				client.EXPECT().LoadChart(gomock.Any()).Return(&chart.Chart{}, nil)
			}

			_, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, "https://example.com", "contour")
			if succeeds {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, tt.wantLog, logged.lines)
		})
	}
}

// Test_HelmChartFromContainerRegistry_RequestTimeout checks that a request to a chart repository times out when its
// response doesn't arrive in time, whether the headers or the body stall, including after a redirect like the one
// that GitHub release downloads follow. The attempt must end at the request timeout, and the pull must retry it
// and succeed.
func Test_HelmChartFromContainerRegistry_RequestTimeout(t *testing.T) {
	originalBackoff, originalTimeout := helmChartPullBackoff, chartPullRequestTimeout
	helmChartPullBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(2, goretry.NewConstant(time.Millisecond))
	}
	chartPullRequestTimeout = time.Second
	t.Cleanup(func() { helmChartPullBackoff, chartPullRequestTimeout = originalBackoff, originalTimeout })
	chartBytes := chartArchive(t, "contour")

	const (
		indexPath    = "/index.yaml"
		archivePath  = "/contour-0.1.0.tgz"
		downloadPath = "/download/contour-0.1.0.tgz"
	)
	tests := []struct {
		name      string
		redirect  bool   // the archive redirects to downloadPath
		stallPath string // the path whose first response stalls
		stallBody bool   // the stalled response sends its headers and half of its body first
	}{
		{name: "no request times out"},
		{name: "index headers time out", stallPath: indexPath},
		{name: "index body times out", stallPath: indexPath, stallBody: true},
		{name: "archive headers time out", stallPath: archivePath},
		{name: "archive body times out", stallPath: archivePath, stallBody: true},
		{name: "redirected archive headers time out", redirect: true, stallPath: downloadPath},
		{name: "redirected archive body times out", redirect: true, stallPath: downloadPath, stallBody: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setUserHelmEnv(t, missingRepositoryIndex)
			logged := recordChartPullLog(t)

			// An attempt requests the index and then the archive, which can redirect.
			attemptPaths := []string{indexPath, archivePath}
			require.Len(t, attemptPaths, chartRepositoryPullRequests)
			if tt.redirect {
				attemptPaths = append(attemptPaths, downloadPath)
			}
			wantPaths, wantPulls := attemptPaths, 1
			if tt.stallPath != "" {
				// The first attempt ends with the request that stalls, and the second attempt makes all its requests.
				wantPaths = append(slices.Clone(attemptPaths[:slices.Index(attemptPaths, tt.stallPath)+1]), attemptPaths...)
				wantPulls = 2
			}

			var mu sync.Mutex
			var paths []string
			var stalled atomic.Bool
			stop := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()

				var body []byte
				switch {
				case r.URL.Path == indexPath:
					body = []byte(contourIndexYAML)
				case r.URL.Path == archivePath && tt.redirect:
					http.Redirect(w, r, downloadPath, http.StatusFound)
					return
				case r.URL.Path == archivePath || (r.URL.Path == downloadPath && tt.redirect):
					body = chartBytes
				default:
					http.NotFound(w, r)
					return
				}

				if r.URL.Path == tt.stallPath && stalled.CompareAndSwap(false, true) {
					if tt.stallBody {
						w.Header().Set("Content-Length", strconv.Itoa(len(body)))
						_, _ = w.Write(body[:len(body)/2])
						_ = http.NewResponseController(w).Flush()
						body = body[len(body)/2:]
					}
					// Finish the response only long after the client should have given up, unless the client or the
					// test ends first.
					select {
					case <-r.Context().Done():
						return
					case <-stop:
						return
					case <-time.After(5 * chartPullRequestTimeout):
					}
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			defer close(stop)

			client := &recordingHelmClient{HelmClient: NewHelmClient()}
			got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, server.URL, "contour")
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, "contour", got.Metadata.Name)
			require.Len(t, client.pulls, wantPulls, "a pull whose request timed out must be retried")
			assert.NoError(t, client.pulls[len(client.pulls)-1].err)
			if tt.stallPath == "" {
				assert.Empty(t, logged.lines)
			} else {
				requireTimeout(t, client.pulls[0].err)
				assert.GreaterOrEqual(t, client.pulls[0].duration, chartPullRequestTimeout, "the attempt must wait for the request timeout")
				assert.Less(t, client.pulls[0].duration, chartPullAttemptTimeout(), "the attempt must end at the request timeout")
				require.Len(t, logged.lines, 1, "the retry must be shown")
				assert.True(t, strings.HasPrefix(logged.lines[0], "Downloading the contour chart failed (attempt 1), retrying: "), logged.lines[0])
			}
			mu.Lock()
			assert.Equal(t, wantPaths, paths)
			mu.Unlock()
			env.assertUnchanged(t)
		})
	}
}

// Test_HelmChartFromContainerRegistry_UnresponsiveRepository checks that a pull from a chart repository that never
// answers retries the attempts that timed out, and gives up within the retry window and one more attempt.
func Test_HelmChartFromContainerRegistry_UnresponsiveRepository(t *testing.T) {
	originalTimeout, originalWindow := chartPullRequestTimeout, chartPullRetryWindow
	chartPullRequestTimeout, chartPullRetryWindow = 250*time.Millisecond, 2*time.Second
	t.Cleanup(func() { chartPullRequestTimeout, chartPullRetryWindow = originalTimeout, originalWindow })
	env := setUserHelmEnv(t, missingRepositoryIndex)
	logged := recordChartPullLog(t)

	// A pull that times out its requests gives up long before the repository answers.
	stop := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stop:
		case <-time.After(10 * time.Second):
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	defer close(stop)

	client := &recordingHelmClient{HelmClient: NewHelmClient()}
	start := time.Now()
	got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, server.URL, "contour")
	elapsed := time.Since(start)
	assert.Nil(t, got)
	requireTimeout(t, err)
	assert.GreaterOrEqual(t, len(client.pulls), 2, "a pull whose request timed out must be retried")
	assert.Len(t, logged.lines, len(client.pulls)-1, "each retry must be shown, and only retries")
	// Scheduling can delay the end of a pull, but not by seconds.
	assert.Less(t, elapsed, chartPullRetryWindow+chartPullAttemptTimeout()+2*time.Second, "a pull must end within the retry window and one more attempt")
	env.assertUnchanged(t)
}

// Test_HelmChartFromContainerRegistry_OCITimeout checks that requests to an OCI registry time out, that a registry
// that answers each request just in time can't make an attempt take longer than the attempt timeout, also after
// it listed the tags, and that the pull gives up within the retry window and one more attempt.
func Test_HelmChartFromContainerRegistry_OCITimeout(t *testing.T) {
	pullChart := func(version string) func(context.Context, HelmClient) error {
		return func(ctx context.Context, client HelmClient) error {
			_, err := NewHelmAction(client).HelmChartFromContainerRegistry(ctx, version, &helm.Configuration{}, "oci://"+fakeChartRegistryHost+"/charts", "radius")
			return err
		}
	}
	latestRadiusVersion := func(ctx context.Context, client HelmClient) error {
		_, err := (&Impl{Helm: client}).GetLatestRadiusVersion(ctx)
		return err
	}
	// A registry that answers only after an hour stands in for one that stopped answering. Unlike a registry that
	// never answers, it doesn't deadlock the test if the requests don't time out.
	afterAnHour := func(*http.Request) time.Duration { return time.Hour }
	justInTime := func(*http.Request) time.Duration { return chartPullRequestTimeout - time.Second }
	// Each tag list request takes almost the request timeout, so the first request after the tag list starts so late
	// that the attempt deadline ends it before its own request timeout would.
	stallAfterTagList := func(r *http.Request) time.Duration {
		switch {
		case strings.HasSuffix(r.URL.Path, "/tags/list"):
			return chartPullRequestTimeout - time.Second
		case r.URL.Path == "/token":
			return 0
		default:
			return time.Hour
		}
	}
	tests := []struct {
		name        string
		host        string
		repository  string
		delay       func(*http.Request) time.Duration
		pull        func(context.Context, HelmClient) error
		wantAttempt time.Duration // how long each attempt takes
	}{
		{"registry answers after an hour", fakeChartRegistryHost, fakeChartRegistryRepository, afterAnHour, pullChart("0.1.0"), chartPullRequestTimeout},
		{"registry answers each request just within the request timeout", fakeChartRegistryHost, fakeChartRegistryRepository, justInTime, pullChart("0.1.0"), chartPullAttemptTimeout()},
		{"registry lists tags just within the request timeout and then stops answering", fakeChartRegistryHost, fakeChartRegistryRepository, stallAfterTagList, pullChart("~0.1.0"), chartPullAttemptTimeout()},
		{"latest Radius version", "ghcr.io", "radius-project/helm-chart/radius", afterAnHour, latestRadiusVersion, chartPullRequestTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setUserHelmEnv(t, missingRepositoryIndex, unwritableUserCache, registryCredentials(tt.host, fakeChartRegistryPassword))
			setDefaultTransport(t, &slowTransport{next: newFakeChartRegistry(t, tt.host, tt.repository), delay: tt.delay})
			logged := recordChartPullLog(t)

			// Fake time lets the test use the production timeouts and retry window.
			synctest.Test(t, func(t *testing.T) {
				client := &recordingHelmClient{HelmClient: NewHelmClient()}
				start := time.Now()
				requireTimeout(t, tt.pull(t.Context(), client))
				assert.GreaterOrEqual(t, len(client.pulls), 3, "the retry window must fit three attempts that time out")
				for _, pull := range client.pulls {
					assert.Equal(t, tt.wantAttempt, pull.duration, "an attempt must end at its timeout")
				}
				assert.LessOrEqual(t, time.Since(start), chartPullRetryWindow+chartPullAttemptTimeout(), "a pull must end within the retry window and one more attempt")
				assert.Len(t, logged.lines, len(client.pulls)-1, "each retry must be shown, and only retries")
			})
			env.assertUnchanged(t)
		})
	}
}

func Test_HelmChartFromContainerRegistry_RefusedConnection(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	chartURL := "http://" + listener.Addr().String() + "/chart.tgz"
	require.NoError(t, listener.Close())

	httpGetter, err := getter.NewHTTPGetter(getter.WithTimeout(time.Second))
	require.NoError(t, err)
	_, fetchErr := httpGetter.Get(chartURL)
	require.Error(t, fetchErr)
	require.True(t, isTransientHelmSocketError(fetchErr), "real refused connection must be retryable: %v", fetchErr)
	require.True(t, isTransientHelmChartError(fetchErr))

	originalBackoff := helmChartPullBackoff
	helmChartPullBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(1, goretry.NewConstant(time.Millisecond))
	}
	t.Cleanup(func() { helmChartPullBackoff = originalBackoff })

	client := NewMockHelmClient(gomock.NewController(t))
	gomock.InOrder(
		client.EXPECT().RunHelmPull(gomock.Any(), "contour").Return("", fetchErr),
		client.EXPECT().RunHelmPull(gomock.Any(), "contour").
			DoAndReturn(func(opts []helm.PullOpt, ref string) (string, error) {
				pull := helm.NewPull(opts...)
				require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, "chart.tgz"), []byte("chart"), 0600))
				return "", nil
			}),
	)
	want := &chart.Chart{}
	client.EXPECT().LoadChart(gomock.Any()).Return(want, nil).Times(1)
	got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, "https://example.com", "contour")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func Test_isTransientHelmSocketError_RealReset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Errorf("hijack connection: %v", err)
			return
		}
		tcpConn, ok := conn.(*net.TCPConn)
		if !ok {
			t.Errorf("expected TCP connection, got %T", conn)
		} else if err := tcpConn.SetLinger(0); err != nil {
			t.Errorf("set reset on close: %v", err)
		}
		if err := conn.Close(); err != nil {
			t.Errorf("close connection: %v", err)
		}
	}))
	defer server.Close()
	httpGetter, err := getter.NewHTTPGetter(getter.WithTimeout(time.Second))
	require.NoError(t, err)
	_, fetchErr := httpGetter.Get(server.URL + "/chart.tgz")
	require.Error(t, fetchErr)
	require.True(t, isTransientHelmSocketError(fetchErr), "real reset connection must be retryable: %v", fetchErr)
	require.True(t, isTransientHelmChartError(fetchErr))
}

func Test_HelmChartPull_Cancellation(t *testing.T) {
	originalBackoff := helmChartPullBackoff
	helmChartPullBackoff = func() goretry.Backoff {
		return goretry.WithMaxRetries(2, goretry.NewConstant(time.Hour))
	}
	t.Cleanup(func() { helmChartPullBackoff = originalBackoff })

	tests := []struct {
		name string
		pull func(context.Context, HelmClient) error
	}{
		{"direct pull", func(ctx context.Context, client HelmClient) error {
			_, err := NewHelmAction(client).HelmChartFromContainerRegistry(ctx, "0.1.0", &helm.Configuration{}, "https://example.com", "contour")
			return err
		}},
		{"Contour preparation", func(ctx context.Context, client HelmClient) error {
			_, _, _, err := prepareContourChart(ctx, NewHelmAction(client), NewDefaultClusterOptions().Contour, "")
			return err
		}},
		{"install", func(ctx context.Context, client HelmClient) error {
			return (&Impl{Helm: client}).InstallRadius(ctx, NewDefaultClusterOptions(), "")
		}},
		{"upgrade", func(ctx context.Context, client HelmClient) error {
			return (&Impl{Helm: client}).UpgradeRadius(ctx, NewDefaultClusterOptions(), "")
		}},
		{"latest version", func(ctx context.Context, client HelmClient) error {
			_, err := (&Impl{Helm: client}).GetLatestRadiusVersion(ctx)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, alreadyCanceled := range []bool{true, false} {
				t.Run(fmt.Sprintf("already canceled=%t", alreadyCanceled), func(t *testing.T) {
					synctest.Test(t, func(t *testing.T) {
						ctx, cancel := context.WithCancel(t.Context())
						defer cancel()
						client := NewMockHelmClient(gomock.NewController(t))
						if alreadyCanceled {
							cancel()
						} else {
							client.EXPECT().RunHelmPull(gomock.Any(), gomock.Any()).
								DoAndReturn(func([]helm.PullOpt, string) (string, error) {
									go func() {
										time.Sleep(time.Second)
										cancel()
									}()
									return "", &url.Error{Op: "Get", URL: "https://example.com", Err: io.EOF}
								}).Times(1)
						}

						start := time.Now()
						err := tt.pull(ctx, client)
						require.ErrorIs(t, err, context.Canceled)
						if alreadyCanceled {
							assert.Zero(t, time.Since(start))
						} else {
							assert.Equal(t, time.Second, time.Since(start), "cancellation must stop the one-hour retry wait")
						}
					})
				})
			}
		})
	}
}

func Test_HelmChartFromContainerRegistry_LoadFailure(t *testing.T) {
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmPull(gomock.Any(), "contour").
		DoAndReturn(func(opts []helm.PullOpt, ref string) (string, error) {
			pull := helm.NewPull(opts...)
			require.NoError(t, os.WriteFile(filepath.Join(pull.DestDir, "chart.tgz"), []byte("invalid chart"), 0600))
			return "", nil
		}).Times(1)
	loadErr := io.ErrUnexpectedEOF
	client.EXPECT().LoadChart(gomock.Any()).Return(nil, loadErr).Times(1)
	got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, "https://example.com", "contour")
	assert.Nil(t, got)
	assert.ErrorIs(t, err, loadErr)
}

func Test_HelmChartFromContainerRegistry_GHCRForbidden(t *testing.T) {
	client := NewMockHelmClient(gomock.NewController(t))
	client.EXPECT().RunHelmPull(gomock.Any(), "oci://ghcr.io/radius-project/helm-chart/radius").
		Return("", &errcode.ErrorResponse{StatusCode: http.StatusForbidden, URL: &url.URL{Host: "ghcr.io"}}).
		Times(1)
	got, err := NewHelmAction(client).HelmChartFromContainerRegistry(t.Context(), "0.1.0", &helm.Configuration{}, "oci://ghcr.io/radius-project/helm-chart", "radius")
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docker logout ghcr.io")
}

func Test_isHelmGHCR403Error(t *testing.T) {
	var err error
	var result bool

	err = errors.New("error")
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", errors.New("error"))
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", &errcode.ErrorResponse{})
	result = isHelmGHCR403Error(err)
	assert.False(t, result)

	err = fmt.Errorf("%w: wrapped error", &errcode.ErrorResponse{StatusCode: http.StatusForbidden, URL: &url.URL{Host: "ghcr.io", Path: "/myregistry"}})
	result = isHelmGHCR403Error(err)
	assert.True(t, result)

	err = &errcode.ErrorResponse{StatusCode: http.StatusForbidden, URL: &url.URL{Host: "ghcr.io", Path: "/myregistry"}}
	result = isHelmGHCR403Error(err)
	assert.True(t, result)

	err = &errcode.ErrorResponse{StatusCode: http.StatusUnauthorized, URL: &url.URL{Host: "ghcr.io", Path: "/myregistry"}}
	result = isHelmGHCR403Error(err)
	assert.False(t, result)
}

func Test_parseUserValuesFromCLI(t *testing.T) {
	options := &RadiusChartOptions{
		SetArgs:     []string{"global.zipkin.url=url,global.prometheus.path=path"},
		SetFileArgs: []string{"global.rootCA.cert=./testdata/fake-ca-cert.crt"},
	}

	values, err := parseUserValuesFromCLI(options)
	require.NoError(t, err)

	_, ok := values["global"]
	assert.True(t, ok)
	global := values["global"].(map[string]any)
	_, ok = global["zipkin"]
	assert.True(t, ok)
	zipkin := global["zipkin"].(map[string]any)
	_, ok = zipkin["url"]
	assert.True(t, ok)
	assert.Equal(t, zipkin["url"], "url")
	rootCA := global["rootCA"].(map[string]any)
	_, ok = rootCA["cert"]
	assert.True(t, ok)
	assert.Contains(t, rootCA["cert"], "-----BEGIN CERTIFICATE-----")

	_, ok = global["prometheus"]
	assert.True(t, ok)
	prometheus := global["prometheus"].(map[string]any)
	_, ok = prometheus["path"]
	assert.True(t, ok)
	assert.Equal(t, prometheus["path"], "path")
}

func Test_prepareRadiusChart_DoesNotMutateChartValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	helmChart := &chart.Chart{
		Values: map[string]any{
			"global": map[string]any{
				"existing": "untouched",
			},
		},
	}
	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("test-chart").Return(helmChart, nil).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := &RadiusChartOptions{
		Namespace: "radius-system",
		ChartPath: "test-chart",
		SetArgs:   []string{"global.zipkin.url=url"},
	}

	_, _, values, err := prepareRadiusChart(t.Context(), helmAction, *options, "")
	require.NoError(t, err)

	global, ok := values["global"].(map[string]any)
	require.True(t, ok)
	zipkin, ok := global["zipkin"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "url", zipkin["url"])

	chartGlobal, ok := helmChart.Values["global"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "untouched", chartGlobal["existing"])
	_, hasZipkin := chartGlobal["zipkin"]
	assert.False(t, hasZipkin, "prepareRadiusChart must not mutate helmChart.Values")
}

func Test_parseUserValuesFromCLI_InvalidSetArg(t *testing.T) {
	options := &RadiusChartOptions{
		SetArgs: []string{"invalid_no_equals"},
	}

	_, err := parseUserValuesFromCLI(options)
	require.Error(t, err)
}

func Test_parseUserValuesFromCLI_InvalidSetFileArg(t *testing.T) {
	options := &RadiusChartOptions{
		SetFileArgs: []string{"key=./testdata/nonexistent-file.txt"},
	}

	_, err := parseUserValuesFromCLI(options)
	require.Error(t, err)
}

func Test_parseUserValuesFromCLI_EmptyArgs(t *testing.T) {
	options := &RadiusChartOptions{
		ChartOptions: ChartOptions{},
	}

	values, err := parseUserValuesFromCLI(options)
	require.NoError(t, err)
	assert.Empty(t, values)
}

func Test_prepareRadiusChart_LoadChartError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("bad-chart").Return(nil, errors.New("chart not found")).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := &RadiusChartOptions{
		Namespace: "radius-system",
		ChartPath: "bad-chart",
	}

	_, _, _, err := prepareRadiusChart(t.Context(), helmAction, *options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load Helm chart")
}

func Test_prepareRadiusChart_ParseValuesError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	mockHelmClient.EXPECT().LoadChart("test-chart").Return(&chart.Chart{}, nil).Times(1)
	helmAction := NewHelmAction(mockHelmClient)

	options := &RadiusChartOptions{
		Namespace: "radius-system",
		ChartPath: "test-chart",
		SetArgs:   []string{"invalid_no_equals"},
	}

	_, _, _, err := prepareRadiusChart(t.Context(), helmAction, *options, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse Radius values")
}

func Test_AddRadiusValuesOverrideWithSet(t *testing.T) {
	options := &RadiusChartOptions{
		SetArgs: []string{"rp.image=ghcr.io/radius-project/applications-rp,rp.tag=latest", "global.zipkin.url=url,global.prometheus.path=path"},
	}

	values, err := parseUserValuesFromCLI(options)
	require.NoError(t, err)

	// validate image, tag for rp should have been overridden with latest
	o := values["rp"].(map[string]any)
	_, ok := o["tag"]
	assert.True(t, ok)
	assert.Equal(t, o["tag"], "latest")
	_, ok = o["image"]
	assert.True(t, ok)
	assert.Equal(t, o["image"], "ghcr.io/radius-project/applications-rp")

	_, ok = values["global"]
	assert.True(t, ok)
	global := values["global"].(map[string]any)
	_, ok = global["zipkin"]
	assert.True(t, ok)
	zipkin := global["zipkin"].(map[string]any)
	_, ok = zipkin["url"]
	assert.True(t, ok)
	assert.Equal(t, zipkin["url"], "url")

	_, ok = global["prometheus"]
	assert.True(t, ok)
	prometheus := global["prometheus"].(map[string]any)
	_, ok = prometheus["path"]
	assert.True(t, ok)
	assert.Equal(t, prometheus["path"], "path")
}

// Test_ApplyHelmChart_TimeoutPropagation verifies the caller-configured readiness timeout
// (`rad install kubernetes --timeout`) reaches the Helm client for both the install and the
// reinstall/upgrade path. See https://github.com/radius-project/radius/issues/10236.
func Test_ApplyHelmChart_TimeoutPropagation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reinstall bool
		timeout   time.Duration
	}{
		{
			name:      "install propagates an explicit timeout",
			reinstall: false,
			timeout:   42 * time.Minute,
		},
		{
			name:      "install propagates the unset timeout unchanged",
			reinstall: false,
			timeout:   0,
		},
		{
			name:      "reinstall propagates an explicit timeout",
			reinstall: true,
			timeout:   42 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHelmClient := NewMockHelmClient(ctrl)
			helmAction := NewHelmAction(mockHelmClient)

			helmConf := &helm.Configuration{}
			helmChart := &chart.Chart{}
			vals := map[string]any{"key": "value"}

			existingRelease := &releasev1.Release{
				Name:  "myrelease",
				Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
				Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
			}

			if tt.reinstall {
				mockHelmClient.EXPECT().
					RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
					Return(existingRelease, nil).Times(1)
				mockHelmClient.EXPECT().
					RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true, tt.timeout).
					Return(existingRelease, nil).Times(1)
			} else {
				mockHelmClient.EXPECT().
					RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
					Return(nil, driver.ErrReleaseNotFound).Times(1)
				mockHelmClient.EXPECT().
					RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, tt.timeout).
					Return(existingRelease, nil).Times(1)
			}

			err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
				ReleaseName: "myrelease",
				Namespace:   "myns",
				Wait:        true,
				Reinstall:   tt.reinstall,
				Timeout:     tt.timeout,
			}, vals)
			require.NoError(t, err)
		})
	}
}

func Test_ApplyHelmChart_InstallError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	// QueryRelease: not installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(nil, driver.ErrReleaseNotFound).Times(1)

	// Install returns an error
	mockHelmClient.EXPECT().
		RunHelmInstall(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, gomock.Any()).
		Return(nil, errors.New("install failed")).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
	}, vals)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run Helm install")
}

func Test_ApplyHelmChart_ReinstallPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	existingRelease := &releasev1.Release{
		Name:  "myrelease",
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
	}

	// QueryRelease: already installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(existingRelease, nil).Times(1)

	// Reinstall triggers upgrade with reuseValues=true
	mockHelmClient.EXPECT().
		RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true, gomock.Any()).
		Return(existingRelease, nil).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
		Reinstall:   true,
	}, vals)
	require.NoError(t, err)
}

func Test_ApplyHelmChart_ReinstallError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	existingRelease := &releasev1.Release{
		Name:  "myrelease",
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
	}

	// QueryRelease: already installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(existingRelease, nil).Times(1)

	// Reinstall triggers upgrade but fails
	mockHelmClient.EXPECT().
		RunHelmUpgrade(gomock.AssignableToTypeOf(&helm.Configuration{}), helmChart, vals, "myrelease", "myns", true, true, gomock.Any()).
		Return(nil, errors.New("upgrade failed")).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
		Reinstall:   true,
	}, vals)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to run Helm upgrade")
}

func Test_ApplyHelmChart_AlreadyInstalled_NoReinstall(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{"key": "value"}

	existingRelease := &releasev1.Release{
		Name:  "myrelease",
		Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}},
		Info:  &releasev1.Info{Status: releasecommon.StatusDeployed},
	}

	// QueryRelease: already installed
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(existingRelease, nil).Times(1)

	// No install or upgrade should be called
	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
		Wait:        true,
		Reinstall:   false,
	}, vals)
	require.NoError(t, err)
}

func Test_ApplyHelmChart_QueryReleaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHelmClient := NewMockHelmClient(ctrl)
	helmAction := NewHelmAction(mockHelmClient)

	helmConf := &helm.Configuration{}
	helmChart := &chart.Chart{}
	vals := map[string]any{}

	// QueryRelease returns an error (not ErrReleaseNotFound)
	mockHelmClient.EXPECT().
		RunHelmGet(gomock.AssignableToTypeOf(&helm.Configuration{}), "myrelease").
		Return(nil, errors.New("connection refused")).Times(1)

	err := helmAction.ApplyHelmChart("", helmChart, helmConf, ChartOptions{
		ReleaseName: "myrelease",
		Namespace:   "myns",
	}, vals)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to query Helm release")
}
