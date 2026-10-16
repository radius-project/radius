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

package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"
)

func TestDownloadTerraformOptionsValidate(t *testing.T) {
	t.Parallel()

	valid := downloadTerraformOptions{version: "1.16.5", dir: "/terraform/.terraform-global", timeout: time.Minute}

	tests := []struct {
		name    string
		modify  func(*downloadTerraformOptions)
		wantErr string
	}{
		{name: "valid", modify: func(*downloadTerraformOptions) {}},
		{name: "missing version", modify: func(o *downloadTerraformOptions) { o.version = "" }, wantErr: "version is required"},
		{name: "invalid version", modify: func(o *downloadTerraformOptions) { o.version = "latest" }, wantErr: "invalid Terraform version"},
		{name: "missing dir", modify: func(o *downloadTerraformOptions) { o.dir = "" }, wantErr: "dir is required"},
		{name: "non-positive timeout", modify: func(o *downloadTerraformOptions) { o.timeout = 0 }, wantErr: "timeout must be greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := valid
			tt.modify(&o)
			v, err := o.validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "1.16.5", v.String())
		})
	}
}

func TestDownloadTerraformCommandFlags(t *testing.T) {
	t.Parallel()

	cmd := newDownloadTerraformCommand()
	require.NoError(t, cmd.ParseFlags([]string{"--version=1.16.5", "--dir=/a", "--legacy-dir=/b", "--timeout=1m"}))

	for flag, want := range map[string]string{"version": "1.16.5", "dir": "/a", "legacy-dir": "/b", "timeout": "1m0s"} {
		got, err := cmd.Flags().GetString(flag)
		if flag == "timeout" {
			var d time.Duration
			d, err = cmd.Flags().GetDuration(flag)
			got = d.String()
		}
		require.NoError(t, err)
		require.Equal(t, want, got, flag)
	}
}

func fakeTerraformDownloader(t *testing.T, gotVersion *string) terraformDownloader {
	return func(_ context.Context, v *version.Version, dst string) error {
		*gotVersion = v.String()
		return os.WriteFile(dst, []byte("terraform"), 0755)
	}
}

func TestDownloadTerraform(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	o := downloadTerraformOptions{dir: filepath.Join(root, ".terraform-global"), legacyDir: root}
	v := version.Must(version.NewVersion("1.16.5"))

	var gotVersion, verified string
	verify := func(_ context.Context, path string) error {
		verified = path
		return nil
	}

	require.NoError(t, downloadTerraform(context.Background(), o, v, logr.Discard(), fakeTerraformDownloader(t, &gotVersion), verify))

	require.Equal(t, "1.16.5", gotVersion)
	require.Equal(t, filepath.Join(o.dir, "terraform"), verified)
	require.FileExists(t, filepath.Join(o.dir, ".terraform-ready"))

	legacy, err := os.ReadFile(filepath.Join(root, "terraform"))
	require.NoError(t, err)
	require.Equal(t, "terraform", string(legacy))
	source, err := os.ReadFile(filepath.Join(root, ".terraform-source"))
	require.NoError(t, err)
	require.Equal(t, "pre-mounted\n", string(source))
}

func TestDownloadTerraformNoLegacyDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	o := downloadTerraformOptions{dir: filepath.Join(root, ".terraform-global")}
	v := version.Must(version.NewVersion("1.16.5"))
	var gotVersion string

	require.NoError(t, downloadTerraform(context.Background(), o, v, logr.Discard(), fakeTerraformDownloader(t, &gotVersion), func(context.Context, string) error { return nil }))

	require.FileExists(t, filepath.Join(o.dir, ".terraform-ready"))
	require.NoFileExists(t, filepath.Join(root, "terraform"))
	require.NoFileExists(t, filepath.Join(root, ".terraform-source"))
}

func TestDownloadTerraformFailuresDoNotWriteMarker(t *testing.T) {
	t.Parallel()

	v := version.Must(version.NewVersion("1.16.5"))
	var gotVersion string

	tests := []struct {
		name     string
		download terraformDownloader
		verify   terraformVerifier
		wantErr  string
	}{
		{
			name:     "download fails",
			download: func(context.Context, *version.Version, string) error { return errors.New("network down") },
			verify:   func(context.Context, string) error { return nil },
			wantErr:  "network down",
		},
		{
			name:     "verify fails",
			download: fakeTerraformDownloader(t, &gotVersion),
			verify:   func(context.Context, string) error { return errors.New("exec format error") },
			wantErr:  "failed to verify Terraform binary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			o := downloadTerraformOptions{dir: filepath.Join(root, ".terraform-global"), legacyDir: root}

			err := downloadTerraform(context.Background(), o, v, logr.Discard(), tt.download, tt.verify)
			require.ErrorContains(t, err, tt.wantErr)
			require.NoFileExists(t, filepath.Join(o.dir, ".terraform-ready"))
			require.NoFileExists(t, filepath.Join(root, ".terraform-source"))
		})
	}
}
