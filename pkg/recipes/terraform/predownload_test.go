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

package terraform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_PreDownloadService_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		failures     int
		wantAttempts int
	}{
		{name: "succeeds first attempt", failures: 0, wantAttempts: 1},
		{name: "succeeds after retries", failures: 2, wantAttempts: 3},
		{name: "gives up without error", failures: 100, wantAttempts: preDownloadMaxAttempts},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attempts := 0
			svc := &PreDownloadService{
				initialDelay: time.Millisecond,
				ensure: func(ctx context.Context) error {
					attempts++
					if attempts <= tt.failures {
						return errors.New("download failed")
					}
					return nil
				},
			}

			require.NoError(t, svc.Run(context.Background()))
			require.Equal(t, tt.wantAttempts, attempts)
		})
	}

	t.Run("stops when cancelled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		svc := &PreDownloadService{
			initialDelay: time.Hour,
			ensure: func(ctx context.Context) error {
				attempts++
				cancel()
				return errors.New("download failed")
			},
		}

		require.NoError(t, svc.Run(ctx))
		require.Equal(t, 1, attempts)
	})
}

func Test_copyExecutable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "global", "terraform")
	require.NoError(t, os.WriteFile(src, []byte("binary"), 0600))
	require.NoError(t, os.MkdirAll(filepath.Dir(dst), 0755))

	require.NoError(t, copyExecutable(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "binary", string(data))

	info, err := os.Stat(dst)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0755), info.Mode().Perm())

	entries, err := os.ReadDir(filepath.Dir(dst))
	require.NoError(t, err)
	require.Len(t, entries, 1, "temporary file should be renamed into place")
}

func TestPreDownloadEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"true", true},
		{"1", true},
		{"false", false},
		{"invalid", false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv(PreDownloadEnvVar, tt.value)
			require.Equal(t, tt.want, PreDownloadEnabled())
		})
	}
}
