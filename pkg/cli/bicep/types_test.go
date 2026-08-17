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

package bicep

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/setup"
	"github.com/stretchr/testify/require"
)

func Test_isRemoteURL(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		expected bool
	}{
		{name: "https URL", filePath: "https://example.com/app.bicep", expected: true},
		{name: "http URL", filePath: "http://example.com/app.bicep", expected: true},
		{name: "uppercase scheme", filePath: "HTTPS://example.com/app.bicep", expected: true},
		{name: "malformed but http-prefixed", filePath: "https://[::1/app.bicep", expected: true},
		{name: "relative path", filePath: "app.bicep", expected: false},
		{name: "relative dot path", filePath: "./app.bicep", expected: false},
		{name: "absolute unix path", filePath: "/tmp/app.bicep", expected: false},
		{name: "windows path", filePath: `C:\Users\app.bicep`, expected: false},
		{name: "unsupported scheme", filePath: "ftp://example.com/app.bicep", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, isRemoteURL(tt.filePath))
		})
	}
}

func newTestImpl() *Impl {
	return &Impl{
		FileSystem: filesystem.NewOSFS(),
		Output:     &output.OutputWriter{Writer: io.Discard},
	}
}

// flakyFS wraps a real filesystem but forces specific operations to fail so error branches can be
// exercised. Unset hooks delegate to the embedded filesystem.
type flakyFS struct {
	filesystem.FileSystem
	failMkdirTemp   bool
	failWriteSubstr string
	failReadSubstr  string
}

func (f flakyFS) MkdirTemp(dir, pattern string) (string, error) {
	if f.failMkdirTemp {
		return "", errors.New("mkdirtemp failed")
	}
	return f.FileSystem.MkdirTemp(dir, pattern)
}

func (f flakyFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if f.failWriteSubstr != "" && strings.Contains(name, f.failWriteSubstr) {
		return errors.New("writefile failed")
	}
	return f.FileSystem.WriteFile(name, data, perm)
}

func (f flakyFS) ReadFile(name string) ([]byte, error) {
	if f.failReadSubstr != "" && strings.Contains(name, f.failReadSubstr) {
		return nil, errors.New("readfile failed")
	}
	return f.FileSystem.ReadFile(name)
}

func newBicepServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "resource foo 'Foo' = {}\n")
	}))
	t.Cleanup(server.Close)
	return server
}

func Test_downloadTemplate_Success(t *testing.T) {
	content := []byte("resource foo 'Foo' = {}\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dir/app.bicep", r.URL.Path)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	// Run from an isolated directory with no bicepconfig.json in any parent so the fallback
	// (default Radius config) path is exercised deterministically.
	t.Chdir(t.TempDir())

	i := newTestImpl()
	localPath, cleanup, err := i.downloadTemplate(t.Context(), server.URL+"/dir/app.bicep")
	require.NoError(t, err)
	defer cleanup()

	// The original file name is preserved so compiler diagnostics stay recognizable.
	require.Equal(t, "app.bicep", filepath.Base(localPath))

	got, err := os.ReadFile(localPath)
	require.NoError(t, err)
	require.Equal(t, content, got)

	// With no bicepconfig.json discoverable, the generated default is written alongside.
	configBytes, err := os.ReadFile(filepath.Join(filepath.Dir(localPath), "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, setup.GetVersionedBicepConfig(), string(configBytes))

	// Cleanup removes the temporary directory.
	cleanup()
	_, err = os.Stat(localPath)
	require.True(t, os.IsNotExist(err))
}

func Test_downloadTemplate_CopiesWorkingDirBicepConfig(t *testing.T) {
	content := []byte("resource foo 'Foo' = {}\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(content)
	}))
	defer server.Close()

	// A bicepconfig.json in the working directory should be reused rather than the default.
	workDir := t.TempDir()
	customConfig := `{"extensions":{"radius":"br:example.azurecr.io/radius:custom"}}`
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bicepconfig.json"), []byte(customConfig), 0600))
	t.Chdir(workDir)

	i := newTestImpl()
	localPath, cleanup, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.NoError(t, err)
	defer cleanup()

	configBytes, err := os.ReadFile(filepath.Join(filepath.Dir(localPath), "bicepconfig.json"))
	require.NoError(t, err)
	require.Equal(t, customConfig, string(configBytes))
}

func Test_downloadTemplate_ExceedsMaxSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("this body is larger than the limit"))
	}))
	defer server.Close()

	original := maxRemoteTemplateSize
	maxRemoteTemplateSize = 10
	defer func() { maxRemoteTemplateSize = original }()

	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds the maximum allowed size")
}

func Test_downloadTemplate_JSONHasNoBicepConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"resources":[]}`)
	}))
	defer server.Close()

	i := newTestImpl()
	localPath, cleanup, err := i.downloadTemplate(t.Context(), server.URL+"/template.json")
	require.NoError(t, err)
	defer cleanup()

	// ARM JSON templates do not use bicepconfig.json, so none is written.
	_, err = os.Stat(filepath.Join(filepath.Dir(localPath), "bicepconfig.json"))
	require.True(t, os.IsNotExist(err))
}

func Test_findBicepConfig(t *testing.T) {
	fs := filesystem.NewOSFS()

	// Config discovered in a parent directory of the start directory.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "bicepconfig.json"), []byte("{}"), 0600))
	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0755))
	require.Equal(t, filepath.Join(root, "bicepconfig.json"), findBicepConfig(fs, nested))

	// No config anywhere up the tree from an isolated directory.
	isolated := filepath.Join(t.TempDir(), "sub")
	require.NoError(t, os.MkdirAll(isolated, 0755))
	require.Equal(t, "", findBicepConfig(fs, isolated))
}

func Test_downloadTemplate_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/missing.bicep")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status")
	require.Contains(t, err.Error(), "404")
}

func Test_downloadTemplate_UnsupportedExtension(t *testing.T) {
	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), "https://example.com/app.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "must reference a .json or .bicep file")
}

func Test_downloadTemplate_DownloadFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL + "/app.bicep"
	server.Close() // Close immediately so the connection is refused.

	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), url)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to download template")
}

func Test_PrepareTemplate_RemoteJSON(t *testing.T) {
	template := `{"$schema":"https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#","resources":[]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, template)
	}))
	defer server.Close()

	i := newTestImpl()
	result, err := i.PrepareTemplate(t.Context(), server.URL+"/template.json")
	require.NoError(t, err)
	require.Equal(t, "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#", result["$schema"])
	require.Empty(t, result["resources"])
}

func Test_PrepareTemplate_RemoteUnsupportedExtension(t *testing.T) {
	i := newTestImpl()
	_, err := i.PrepareTemplate(t.Context(), "https://example.com/app.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "must reference a .json or .bicep file")
}

func Test_PrepareTemplate_RemoteJSONInvalidMentionsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{ not valid json")
	}))
	defer server.Close()

	url := server.URL + "/template.json"
	i := newTestImpl()
	_, err := i.PrepareTemplate(t.Context(), url)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read remote template")
	require.Contains(t, err.Error(), url)
}

func Test_downloadTemplate_MkdirTempError(t *testing.T) {
	server := newBicepServer(t)
	i := &Impl{
		FileSystem: flakyFS{FileSystem: filesystem.NewOSFS(), failMkdirTemp: true},
		Output:     &output.OutputWriter{Writer: io.Discard},
	}
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.ErrorContains(t, err, "failed to create temporary directory")
}

func Test_downloadTemplate_WriteTemplateError(t *testing.T) {
	server := newBicepServer(t)
	i := &Impl{
		FileSystem: flakyFS{FileSystem: filesystem.NewOSFS(), failWriteSubstr: "app.bicep"},
		Output:     &output.OutputWriter{Writer: io.Discard},
	}
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.ErrorContains(t, err, "failed to write remote template")
}

func Test_downloadTemplate_WriteBicepConfigError(t *testing.T) {
	// Isolated working dir forces the fallback (generated) config write, which is made to fail.
	t.Chdir(t.TempDir())
	server := newBicepServer(t)
	i := &Impl{
		FileSystem: flakyFS{FileSystem: filesystem.NewOSFS(), failWriteSubstr: "bicepconfig.json"},
		Output:     &output.OutputWriter{Writer: io.Discard},
	}
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.ErrorContains(t, err, "failed to write bicepconfig.json")
}

func Test_downloadTemplate_ReadWorkingDirConfigError(t *testing.T) {
	// A bicepconfig.json exists in the working dir, but reading it fails.
	workDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bicepconfig.json"), []byte("{}"), 0600))
	t.Chdir(workDir)

	server := newBicepServer(t)
	i := &Impl{
		FileSystem: flakyFS{FileSystem: filesystem.NewOSFS(), failReadSubstr: "bicepconfig.json"},
		Output:     &output.OutputWriter{Writer: io.Discard},
	}
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.ErrorContains(t, err, "failed to read")
}

func Test_downloadTemplate_MalformedURL(t *testing.T) {
	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), "https://[::1/app.bicep")
	require.ErrorContains(t, err, "invalid template URL")
}

func Test_downloadTemplate_MissingHost(t *testing.T) {
	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), "https:///app.bicep")
	require.ErrorContains(t, err, "missing host")
}

func Test_PrepareTemplate_MalformedURLNotTreatedAsLocal(t *testing.T) {
	// A malformed http(s) URL must surface a URL error, not a misleading local file-not-found.
	i := newTestImpl()
	_, err := i.PrepareTemplate(t.Context(), "https://[::1/app.bicep")
	require.ErrorContains(t, err, "invalid template URL")
	require.NotContains(t, err.Error(), "could not find file")
}

func Test_downloadTemplate_RejectsHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><body>not a template</body></html>")
	}))
	defer server.Close()

	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.ErrorContains(t, err, "HTML page")
}

func Test_downloadTemplate_RetriesTransientFailures(t *testing.T) {
	original := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = original }()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "resource foo 'Foo' = {}\n")
	}))
	defer server.Close()

	t.Chdir(t.TempDir())
	i := newTestImpl()
	_, cleanup, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.NoError(t, err)
	defer cleanup()
	require.Equal(t, int32(3), attempts.Load())
}

func Test_downloadTemplate_NoRetryOn4xx(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), server.URL+"/app.bicep")
	require.Error(t, err)
	require.Equal(t, int32(1), attempts.Load())
}

func Test_downloadTemplate_RedactsCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "http://")
	withCreds := "http://user:supersecret@" + host + "/missing.bicep?sig=TOPSECRET"

	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), withCreds)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "supersecret")
	require.NotContains(t, err.Error(), "TOPSECRET")
	require.Contains(t, err.Error(), "redacted")
}

func Test_downloadTemplate_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "resource foo 'Foo' = {}\n")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel before the request so the download is aborted.

	i := newTestImpl()
	_, _, err := i.downloadTemplate(ctx, server.URL+"/app.bicep")
	require.Error(t, err)
}

func Test_redactURL(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		notContain []string
		contain    []string
	}{
		{
			name:       "userinfo and query",
			raw:        "https://user:secret@example.com/app.bicep?sig=abc123&other=xyz",
			notContain: []string{"secret", "abc123", "xyz"},
			contain:    []string{"redacted", "example.com"},
		},
		{
			name:    "no credentials",
			raw:     "https://example.com/app.bicep",
			contain: []string{"https://example.com/app.bicep"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactURL(tt.raw)
			for _, s := range tt.notContain {
				require.NotContains(t, got, s)
			}
			for _, s := range tt.contain {
				require.Contains(t, got, s)
			}
		})
	}
}

func Test_RedactTemplatePath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		expected   string
		notContain string
	}{
		{
			name:     "local path unchanged",
			path:     "./app.bicep",
			expected: "./app.bicep",
		},
		{
			name:     "local path with query-like name unchanged",
			path:     "/tmp/app.bicep?notaquery",
			expected: "/tmp/app.bicep?notaquery",
		},
		{
			name:     "remote url without credentials unchanged",
			path:     "https://example.com/app.bicep",
			expected: "https://example.com/app.bicep",
		},
		{
			name:       "remote url with signed query redacted",
			path:       "https://example.com/app.bicep?sig=TOPSECRET",
			expected:   "https://example.com/app.bicep?sig=redacted",
			notContain: "TOPSECRET",
		},
		{
			name:       "remote url with userinfo redacted",
			path:       "https://user:" + "TOPSECRET" + "@example.com/app.bicep",
			expected:   "https://redacted@example.com/app.bicep",
			notContain: "TOPSECRET",
		},
		{
			name:       "remote url with fragment dropped",
			path:       "https://example.com/app.bicep#token=" + "TOPSECRET",
			expected:   "https://example.com/app.bicep",
			notContain: "TOPSECRET",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RedactTemplatePath(tt.path)
			require.Equal(t, tt.expected, got)
			if tt.notContain != "" {
				require.NotContains(t, got, tt.notContain)
			}
		})
	}
}

// PrepareTemplate must never surface the raw template argument, because a remote URL can carry a
// SAS token or basic-auth userinfo that would otherwise land in terminal and CI logs.
func Test_PrepareTemplate_RemoteErrorRedactsCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "{ not valid json")
	}))
	defer server.Close()

	i := newTestImpl()
	_, err := i.PrepareTemplate(t.Context(), server.URL+"/template.json?sig=TOPSECRET")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read remote template")
	require.NotContains(t, err.Error(), "TOPSECRET")
	require.Contains(t, err.Error(), "sig=redacted")
}

func Test_TemplateFileName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{name: "local relative path", path: "./app.bicep", expected: "app.bicep"},
		{name: "local absolute path", path: "/tmp/dir/app.bicep", expected: "app.bicep"},
		{name: "remote url", path: "https://example.com/dir/app.bicep", expected: "app.bicep"},
		{
			// filepath.Base would return "app.bicep?sig=abc.def" here, putting part of the
			// credential into an on-disk file name.
			name:     "remote url with dotted query value drops the query",
			path:     "https://example.com/app.bicep?sig=abc.def",
			expected: "app.bicep",
		},
		{name: "remote url with fragment drops the fragment", path: "https://example.com/app.bicep#token=abc", expected: "app.bicep"},
		{name: "malformed url", path: "https://[::1/app.bicep", expected: "template"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.expected, TemplateFileName(tt.path))
		})
	}
}

// A transport failure returns a *url.Error whose message embeds the raw request URL, so the
// wrapped error must not carry the credential that the redacted display string removed.
func Test_downloadTemplate_TransportErrorRedactsCredentials(t *testing.T) {
	i := newTestImpl()
	_, _, err := i.downloadTemplate(t.Context(), "https://nonexistent.invalid/app.bicep?sig=TOPSECRET")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "TOPSECRET")
	require.Contains(t, err.Error(), "sig=redacted")
}

func Test_newHTTPClient_RedirectPolicy(t *testing.T) {
	client := newHTTPClient()
	mustReq := func(rawURL string) *http.Request {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		require.NoError(t, err)
		return req
	}

	// https -> http downgrade is refused.
	err := client.CheckRedirect(mustReq("http://evil.example/app.bicep"), []*http.Request{mustReq("https://good.example/app.bicep")})
	require.ErrorContains(t, err, "refusing to follow redirect")

	// https -> https is allowed.
	err = client.CheckRedirect(mustReq("https://good2.example/app.bicep"), []*http.Request{mustReq("https://good.example/app.bicep")})
	require.NoError(t, err)

	// http -> http is allowed (kubectl compatibility for plain http input).
	err = client.CheckRedirect(mustReq("http://good2.example/app.bicep"), []*http.Request{mustReq("http://good.example/app.bicep")})
	require.NoError(t, err)
}

func Test_parseRetryAfter(t *testing.T) {
	require.Equal(t, 5*time.Second, parseRetryAfter("5"))
	require.Equal(t, time.Duration(0), parseRetryAfter(""))
	require.Equal(t, time.Duration(0), parseRetryAfter("-1"))
	require.Equal(t, time.Duration(0), parseRetryAfter("not-a-number"))

	future := parseRetryAfter(time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat))
	require.Greater(t, future, time.Duration(0))
}
