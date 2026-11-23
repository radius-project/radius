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

package tools

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/radius-project/radius/internal/tooling"
	"github.com/stretchr/testify/require"
)

func TestDownloadToFolder(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		status    int
		truncated bool
		wantErr   string
	}{
		{name: "success", status: http.StatusOK},
		{name: "not found", status: http.StatusNotFound, wantErr: "404 Not Found"},
		{name: "gateway timeout", status: http.StatusGatewayTimeout, wantErr: "504 Gateway Timeout"},
		{name: "partial content", status: http.StatusPartialContent, wantErr: "206 Partial Content"},
		{name: "interrupted download", status: http.StatusOK, truncated: true, wantErr: "unexpected EOF"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			const contents = "downloaded binary"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.truncated {
					w.Header().Set("Content-Length", fmt.Sprint(len(contents)+10))
				}
				w.WriteHeader(tt.status)
				_, err := w.Write([]byte(contents))
				if err != nil {
					t.Errorf("write download response: %v", err)
				}
			}))
			defer server.Close()

			for _, installed := range []bool{false, true} {
				t.Run(fmt.Sprintf("installed=%t", installed), func(t *testing.T) {
					filename := filepath.Join(t.TempDir(), "bin", "bicep")
					const previous = "previous binary"
					if installed {
						require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
						require.NoError(t, os.WriteFile(filename, []byte(previous), 0o755))
					}

					err := downloadToFolder(filename, server.URL)
					if tt.wantErr == "" {
						require.NoError(t, err)
						actual, err := os.ReadFile(filename)
						require.NoError(t, err)
						require.Equal(t, contents, string(actual))
						if runtime.GOOS != "windows" {
							info, err := os.Stat(filename)
							require.NoError(t, err)
							require.Equal(t, os.FileMode(0o755), info.Mode().Perm())
						}
					} else {
						require.ErrorContains(t, err, tt.wantErr)
						require.ErrorContains(t, err, server.URL)
						if installed {
							actual, err := os.ReadFile(filename)
							require.NoError(t, err)
							require.Equal(t, previous, string(actual))
						} else {
							_, err := os.Stat(filename)
							require.ErrorIs(t, err, os.ErrNotExist)
						}
					}

					temporaryFiles, err := filepath.Glob(filepath.Join(filepath.Dir(filename), ".bicep-*"))
					require.NoError(t, err)
					require.Empty(t, temporaryFiles)
				})
			}
		})
	}
}

func TestBicepToolManifest(t *testing.T) {
	t.Parallel()

	manifest, err := tooling.LoadManifest(filepath.Join("..", "..", "..", "..", "build", "tools.yaml"))
	require.NoError(t, err)

	for _, tool := range manifest.Tools {
		if tool.Name != "bicep" {
			continue
		}

		require.Equal(t, tool.Version, bicepVersion, "the build and distributed CLI must pin the same Bicep version")
		for platform, entry := range tool.Platforms {
			t.Run(platform, func(t *testing.T) {
				t.Parallel()

				currentOS, currentArch, ok := strings.Cut(platform, "_")
				require.True(t, ok)
				asset, err := GetValidPlatform(currentOS, currentArch)
				require.NoError(t, err)
				require.Equal(t, entry.Asset, asset, "the installer and distributed CLI must download the same asset")
				values, err := tool.TemplateValues(platform, tool.Version)
				require.NoError(t, err)
				downloadURL, err := tooling.ExpandTemplate(tool.DownloadTemplate, values)
				require.NoError(t, err)
				require.Equal(t, downloadURL, binaryRepo+asset)
			})
		}
		return
	}

	t.Fatal("Bicep is missing from build/tools.yaml")
}

func TestGetValidPlatform(t *testing.T) {
	osArchTests := []struct {
		currentOS   string
		currentArch string
		out         string
		err         error
	}{
		{
			currentOS:   "darwin",
			currentArch: "amd64",
			out:         "bicep-osx-x64",
		},
		{
			currentOS:   "darwin",
			currentArch: "arm64",
			out:         "bicep-osx-arm64",
		},
		{
			currentOS:   "windows",
			currentArch: "amd64",
			out:         "bicep-win-x64",
		},
		{
			currentOS:   "windows",
			currentArch: "arm64",
			out:         "bicep-win-arm64",
		},
		{
			currentOS:   "linux",
			currentArch: "amd64",
			out:         "bicep-linux-x64",
		},
		{
			currentOS:   "linux",
			currentArch: "arm",
			out:         "",
			err:         errors.New("unsupported platform linux/arm"),
		},
		{
			currentOS:   "linux",
			currentArch: "arm64",
			out:         "bicep-linux-arm64",
		},
	}

	for _, tc := range osArchTests {
		t.Run(tc.currentOS+"-"+tc.currentArch, func(t *testing.T) {
			platform, err := GetValidPlatform(tc.currentOS, tc.currentArch)
			if tc.err != nil {
				require.ErrorContains(t, err, err.Error())
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.out, platform, "GetValidPlatform() got = %v, want %v", platform, tc.out)
		})
	}
}
