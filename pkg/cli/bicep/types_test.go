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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/cli/filesystem"
	"github.com/radius-project/radius/pkg/cli/output"
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

func Test_downloadTemplate_Success(t *testing.T) {
	content := []byte("resource foo 'Foo' = {}\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dir/app.bicep", r.URL.Path)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	i := newTestImpl()
	localPath, cleanup, err := i.downloadTemplate(server.URL + "/dir/app.bicep")
	require.NoError(t, err)
	defer cleanup()

	// The original file name is preserved so compiler diagnostics stay recognizable.
	require.Equal(t, "app.bicep", filepath.Base(localPath))

	got, err := os.ReadFile(localPath)
	require.NoError(t, err)
	require.Equal(t, content, got)

	// A bicepconfig.json is written alongside the template so `extension` declarations resolve.
	configBytes, err := os.ReadFile(filepath.Join(filepath.Dir(localPath), "bicepconfig.json"))
	require.NoError(t, err)
	require.Contains(t, string(configBytes), "extensions")
	require.Contains(t, string(configBytes), "radius")

	// Cleanup removes the temporary directory.
	cleanup()
	_, err = os.Stat(localPath)
	require.True(t, os.IsNotExist(err))
}

func Test_downloadTemplate_JSONHasNoBicepConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"resources":[]}`)
	}))
	defer server.Close()

	i := newTestImpl()
	localPath, cleanup, err := i.downloadTemplate(server.URL + "/template.json")
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
	_, _, err := i.downloadTemplate(server.URL + "/missing.bicep")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected status")
	require.Contains(t, err.Error(), "404")
}

func Test_downloadTemplate_UnsupportedExtension(t *testing.T) {
	i := newTestImpl()
	_, _, err := i.downloadTemplate("https://example.com/app.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "must reference a .json or .bicep file")
}

func Test_downloadTemplate_DownloadFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL + "/app.bicep"
	server.Close() // Close immediately so the connection is refused.

	i := newTestImpl()
	_, _, err := i.downloadTemplate(url)
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
	result, err := i.PrepareTemplate(server.URL + "/template.json")
	require.NoError(t, err)
	require.Equal(t, "https://schema.management.azure.com/schemas/2019-04-01/deploymentTemplate.json#", result["$schema"])
	require.Empty(t, result["resources"])
}

func Test_PrepareTemplate_RemoteUnsupportedExtension(t *testing.T) {
	i := newTestImpl()
	_, err := i.PrepareTemplate("https://example.com/app.txt")
	require.Error(t, err)
	require.Contains(t, err.Error(), "must reference a .json or .bicep file")
}
