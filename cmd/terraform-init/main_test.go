/*
Copyright 2026 The Radius Authors.

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

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/pkg/recipes/terraform/tfbinary"
)

const testChecksum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseFlags(t *testing.T) {
	t.Parallel()

	o, err := parseFlags([]string{"--version=1.16.5", "--legacy-dir=/legacy"})
	require.NoError(t, err)
	require.Equal(t, options{version: "1.16.5", dir: tfbinary.DefaultGlobalDir, legacyDir: "/legacy", timeout: defaultDownloadTimeout}, o)

	_, err = parseFlags([]string{"--version=1.16.5", "extra"})
	require.ErrorContains(t, err, "unexpected arguments")

	_, err = parseFlags([]string{"--unknown"})
	require.Error(t, err)
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		opts        options
		wantVersion string
		wantErr     string
	}{
		{name: "version", opts: options{version: "1.16.5"}, wantVersion: "1.16.5"},
		{name: "url", opts: options{url: "https://mirror.example.com/terraform.zip"}},
		{name: "url with checksum", opts: options{url: "http://mirror.example.com/terraform.zip", sha256: strings.ToUpper(testChecksum)}},
		{name: "neither", opts: options{}, wantErr: "one of --version or --url is required"},
		{name: "both", opts: options{version: "1.16.5", url: "https://example.com/t.zip"}, wantErr: "mutually exclusive"},
		{name: "checksum without url", opts: options{version: "1.16.5", sha256: testChecksum}, wantErr: "--sha256 can only be used with --url"},
		{name: "invalid version", opts: options{version: "latest"}, wantErr: "invalid Terraform version"},
		{name: "relative url", opts: options{url: "/terraform.zip"}, wantErr: "absolute http or https URL"},
		{name: "unsupported scheme", opts: options{url: "file:///terraform.zip"}, wantErr: "absolute http or https URL"},
		{name: "short checksum", opts: options{url: "https://example.com/t.zip", sha256: "abc"}, wantErr: "64-character hex"},
		{name: "non-hex checksum", opts: options{url: "https://example.com/t.zip", sha256: strings.Repeat("z", 64)}, wantErr: "64-character hex"},
		{name: "missing dir", opts: options{version: "1.16.5", dir: "-"}, wantErr: "--dir is required"},
		{name: "non-positive timeout", opts: options{version: "1.16.5", timeout: -1}, wantErr: "timeout must be greater than zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := tt.opts
			if o.dir == "" {
				o.dir = "/terraform/.terraform-global"
			} else if o.dir == "-" {
				o.dir = ""
			}
			if o.timeout == 0 {
				o.timeout = time.Minute
			}

			v, err := o.validate()
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			if tt.wantVersion == "" {
				require.Nil(t, v)
			} else {
				require.Equal(t, tt.wantVersion, v.String())
			}
		})
	}
}

func fakeDownload(t *testing.T, calls *int) downloader {
	return func(_ context.Context, dst string) error {
		*calls++
		return os.WriteFile(dst, []byte("terraform"), 0755)
	}
}

func fakeVerify(reported string, err error) verifier {
	return func(context.Context, string) (string, error) { return reported, err }
}

func TestInstallTerraform(t *testing.T) {
	t.Parallel()

	v1165 := version.Must(version.NewVersion("1.16.5"))

	tests := []struct {
		name      string
		want      *version.Version
		legacy    bool
		download  func(*testing.T, *int) downloader
		verify    verifier
		wantErr   string
		wantReady string
	}{
		{name: "release", want: v1165, download: fakeDownload, verify: fakeVerify("1.16.5", nil), wantReady: "1.16.5\n"},
		{name: "release with legacy dir", want: v1165, legacy: true, download: fakeDownload, verify: fakeVerify("1.16.5", nil), wantReady: "1.16.5\n"},
		{name: "url records reported version", download: fakeDownload, verify: fakeVerify("1.7.0", nil), wantReady: "1.7.0\n"},
		{name: "version mismatch", want: v1165, download: fakeDownload, verify: fakeVerify("1.7.0", nil), wantErr: "expected version 1.16.5, got 1.7.0"},
		{name: "verify fails", want: v1165, download: fakeDownload, verify: fakeVerify("", errors.New("exec format error")), wantErr: "exec format error"},
		{name: "invalid reported version", download: fakeDownload, verify: fakeVerify("not-a-version", nil), wantErr: "invalid version"},
		{
			name: "download fails",
			want: v1165,
			download: func(_ *testing.T, calls *int) downloader {
				return func(context.Context, string) error { *calls++; return errors.New("network unreachable") }
			},
			verify:  fakeVerify("1.16.5", nil),
			wantErr: "network unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			o := options{dir: filepath.Join(t.TempDir(), "global")}
			if tt.legacy {
				o.legacyDir = t.TempDir()
			}
			calls := 0
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))

			err := installTerraform(context.Background(), o, tt.want, logger, tt.download(t, &calls), tt.verify)
			require.Equal(t, 1, calls)

			marker := filepath.Join(o.dir, tfbinary.MarkerFileName)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.NoFileExists(t, marker, "the ready marker must not be written on failure")
				return
			}
			require.NoError(t, err)

			content, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, tt.wantReady, string(content))

			if tt.legacy {
				info, err := os.Stat(filepath.Join(o.legacyDir, tfbinary.BinaryName))
				require.NoError(t, err)
				require.NotZero(t, info.Mode()&0111)
				source, err := os.ReadFile(filepath.Join(o.legacyDir, legacySourceFile))
				require.NoError(t, err)
				require.Equal(t, "pre-mounted\n", string(source))
			}
		})
	}
}

func TestTerraformVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	write := func(name, script string) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0755))
		return p
	}

	got, err := terraformVersion(context.Background(), write("ok", `echo '{"terraform_version":"1.16.5","platform":"linux_amd64"}'`))
	require.NoError(t, err)
	require.Equal(t, "1.16.5", got)

	_, err = terraformVersion(context.Background(), write("bad-json", "echo Terraform v1.16.5"))
	require.ErrorContains(t, err, "unexpected output")

	_, err = terraformVersion(context.Background(), write("fails", "echo boom >&2; exit 3"))
	require.ErrorContains(t, err, "boom")
}
