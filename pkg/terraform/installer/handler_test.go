/*
Copyright 2024 The Radius Authors.

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

package installer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/stretchr/testify/require"
)

func TestHandleInstall_Succeeds(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	zipBytes := buildZip(t)
	sum := sha256.Sum256(zipBytes)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: zipBytes}},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "http://example.com/terraform.zip",
		Checksum:  checksum,
	})

	require.NoError(t, handler.Handle(ctx, msg))

	status, err := store.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", status.Current)
	vs := status.Versions["1.0.0"]
	require.Equal(t, VersionStateSucceeded, vs.State)
	require.FileExists(t, filepath.Join(tempDir, "versions", "1.0.0", "terraform"))
}

func TestHandleInstall_ChecksumFail(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	zipBytes := buildZip(t)

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: zipBytes}},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "http://example.com/terraform.zip",
		Checksum:  "sha256:deadbeef",
	})

	err := handler.Handle(ctx, msg)
	require.Error(t, err)

	status, _ := store.Get(ctx)
	vs := status.Versions["1.0.0"]
	require.Equal(t, VersionStateFailed, vs.State)
	require.NotEmpty(t, vs.LastError)
	require.Empty(t, status.Current)
}

func TestHandleUninstall(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// seed status with another current version
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "2.0.0",
		Versions: map[string]VersionStatus{
			"2.0.0": {Version: "2.0.0", State: VersionStateSucceeded},
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	targetDir := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "terraform"), []byte("tf"), 0o755))

	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "1.0.0",
	})

	require.NoError(t, handler.Handle(ctx, msg))

	status, _ := store.Get(ctx)
	vs := status.Versions["1.0.0"]
	require.Equal(t, VersionStateUninstalled, vs.State)
	require.NoFileExists(t, filepath.Join(targetDir, "terraform"))
}

func TestHandleInstall_LockContention(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: buildZip(t)}},
	}

	// Pre-create lock to simulate concurrent operation.
	lockPath := filepath.Join(tempDir, ".terraform-installer.lock")
	require.NoError(t, os.MkdirAll(tempDir, 0o755))
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	require.NoError(t, err)
	defer func() {
		_ = lock.Close()
		_ = os.Remove(lockPath)
	}()

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.2.3",
		SourceURL: "http://example.com/terraform.zip",
	})

	err = handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "installer is busy")
}

func TestHandleInstall_StaleLockFailsBusy(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: buildZip(t)}},
	}

	// Create and close lock file to simulate leftover; handler should report busy.
	lockPath := filepath.Join(tempDir, ".terraform-installer.lock")
	require.NoError(t, os.MkdirAll(tempDir, 0o755))
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	require.NoError(t, err)
	_ = lock.Close()

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.2.4",
		SourceURL: "http://example.com/terraform.zip",
	})

	err = handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "installer is busy")
}

func TestHandleInstall_RootPathUnwritable(t *testing.T) {
	ctx := context.Background()

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    "/dev/null/should-fail",
		HTTPClient:  &http.Client{Transport: stubTransport{body: buildZip(t)}},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.2.5",
		SourceURL: "http://example.com/terraform.zip",
	})

	err := handler.Handle(ctx, msg)
	require.Error(t, err)
}

type stubTransport struct {
	body []byte
}

func (t stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(t.body)),
		Header:     make(http.Header),
	}, nil
}

func buildZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("terraform")
	require.NoError(t, err)
	_, err = f.Write([]byte("binary"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func TestIsZipArchive(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
		wantErr bool
	}{
		{
			name:    "valid zip magic bytes",
			content: []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00}, // PK\x03\x04 + extra bytes
			want:    true,
			wantErr: false,
		},
		{
			name:    "real zip file",
			content: nil, // will be filled with actual zip
			want:    true,
			wantErr: false,
		},
		{
			name:    "plain binary (no magic)",
			content: []byte("#!/bin/bash\necho hello"),
			want:    false,
			wantErr: false,
		},
		{
			name:    "ELF binary header",
			content: []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01}, // ELF magic
			want:    false,
			wantErr: false,
		},
		{
			name:    "file too small",
			content: []byte{0x50, 0x4B}, // only 2 bytes
			want:    false,
			wantErr: false,
		},
		{
			name:    "empty file",
			content: []byte{},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			testFile := filepath.Join(tempDir, "testfile")

			content := tt.content
			if tt.name == "real zip file" {
				// Build an actual zip for this test case
				var buf bytes.Buffer
				w := zip.NewWriter(&buf)
				f, _ := w.Create("terraform")
				_, _ = f.Write([]byte("binary"))
				_ = w.Close()
				content = buf.Bytes()
			}

			require.NoError(t, os.WriteFile(testFile, content, 0o644))

			got, err := isZipArchive(testFile)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got, "isZipArchive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsZipArchive_FileNotFound(t *testing.T) {
	_, err := isZipArchive("/nonexistent/path/to/file")
	require.Error(t, err)
}

func TestStageBinary_PlainBinary(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Create a plain binary file (not a zip)
	binaryContent := []byte("#!/bin/bash\necho terraform")
	sourcePath := filepath.Join(tempDir, "terraform-download")
	require.NoError(t, os.WriteFile(sourcePath, binaryContent, 0o644))

	targetPath := filepath.Join(tempDir, "terraform")

	handler := &Handler{RootPath: tempDir}
	err := handler.stageBinary(ctx, sourcePath, targetPath)
	require.NoError(t, err)

	// Verify the file was copied (not extracted)
	content, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, binaryContent, content)
}

func TestStageBinary_ZipArchive(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Create a zip archive without .zip extension (like downloads)
	zipContent := buildZip(t)
	sourcePath := filepath.Join(tempDir, "terraform-download") // no extension!
	require.NoError(t, os.WriteFile(sourcePath, zipContent, 0o644))

	targetPath := filepath.Join(tempDir, "terraform")

	handler := &Handler{RootPath: tempDir}
	err := handler.stageBinary(ctx, sourcePath, targetPath)
	require.NoError(t, err)

	// Verify the binary was extracted
	content, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	require.Equal(t, []byte("binary"), content)
}

func TestHandleInstall_IdempotentSkipsReinstall(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Pre-setup: mark version as already installed in status
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "1.0.0",
		Versions: map[string]VersionStatus{
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create the existing binary
	targetDir := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "terraform"), []byte("existing binary"), 0o755))

	// Setup handler with a stub transport that would fail if called
	downloadCalled := false
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient: &http.Client{Transport: &trackingTransport{
			onRequest: func() { downloadCalled = true },
			body:      buildZip(t),
		}},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "http://example.com/terraform.zip",
	})

	// Should succeed without downloading
	require.NoError(t, handler.Handle(ctx, msg))
	require.False(t, downloadCalled, "download should be skipped for already-installed version")

	// Verify the original binary is unchanged
	content, err := os.ReadFile(filepath.Join(targetDir, "terraform"))
	require.NoError(t, err)
	require.Equal(t, []byte("existing binary"), content)
}

func TestHandleInstall_ReinstallsIfBinaryMissing(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	zipBytes := buildZip(t)
	sum := sha256.Sum256(zipBytes)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	// Pre-setup: mark version as installed but don't create the binary
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "1.0.0",
		Versions: map[string]VersionStatus{
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Don't create the binary - it's "missing"

	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: zipBytes}},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "http://example.com/terraform.zip",
		Checksum:  checksum,
	})

	// Should succeed and reinstall
	require.NoError(t, handler.Handle(ctx, msg))

	// Verify the binary was (re)installed
	require.FileExists(t, filepath.Join(tempDir, "versions", "1.0.0", "terraform"))
}

func TestHandleInstall_PromotesPreviouslyInstalledVersion(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Pre-setup: version 1.2.0 is installed but 1.0.0 is current
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "1.0.0",
		Versions: map[string]VersionStatus{
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
			"1.2.0": {Version: "1.2.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create both version directories with binaries
	targetDir100 := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(targetDir100, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir100, "terraform"), []byte("binary 1.0.0"), 0o755))

	targetDir120 := filepath.Join(tempDir, "versions", "1.2.0")
	require.NoError(t, os.MkdirAll(targetDir120, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir120, "terraform"), []byte("binary 1.2.0"), 0o755))

	// Setup handler with a tracking transport to verify no download happens
	downloadCalled := false
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient: &http.Client{Transport: &trackingTransport{
			onRequest: func() { downloadCalled = true },
			body:      buildZip(t),
		}},
	}

	// Request install of 1.2.0 (already installed but not current)
	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.2.0",
		SourceURL: "http://example.com/terraform.zip",
	})

	// Should succeed without downloading
	require.NoError(t, handler.Handle(ctx, msg))
	require.False(t, downloadCalled, "download should be skipped for already-installed version")

	// Verify status was updated to promote 1.2.0
	status, err := store.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.2.0", status.Current, "current version should be promoted to 1.2.0")
	require.Equal(t, "1.0.0", status.Previous, "previous version should be 1.0.0")

	// Verify symlink points to 1.2.0
	symlinkPath := filepath.Join(tempDir, "current")
	target, err := os.Readlink(symlinkPath)
	require.NoError(t, err)
	require.Contains(t, target, "1.2.0", "symlink should point to 1.2.0")
}

type trackingTransport struct {
	onRequest func()
	body      []byte
}

func (t *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.onRequest != nil {
		t.onRequest()
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader(t.body)),
		Header:     make(http.Header),
	}, nil
}

func TestHandleInstall_PathTraversalRejected(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: buildZip(t)}},
	}

	// Try to install with a path traversal version
	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "../../../etc/malicious",
		SourceURL: "http://example.com/terraform.zip",
	})

	err := handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path traversal")

	// Verify malicious directory was not created
	require.NoFileExists(t, filepath.Join(tempDir, "..", "..", "..", "etc", "malicious"))
}

func TestHandleUninstall_PathTraversalRejected(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
	}

	// Try to uninstall with a path traversal version
	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "../../../etc/passwd",
	})

	err := handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path traversal")
}

// mockExecutionChecker is a test helper that implements ExecutionChecker
type mockExecutionChecker struct {
	active bool
	err    error
}

func (m *mockExecutionChecker) HasActiveExecutions(ctx context.Context) (bool, error) {
	return m.active, m.err
}

func TestHandleUninstall_BlockedByActiveExecutions(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup: install a version first
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "2.0.0",
		Versions: map[string]VersionStatus{
			"2.0.0": {Version: "2.0.0", State: VersionStateSucceeded},
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create the version directory
	targetDir := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "terraform"), []byte("tf"), 0o755))

	// Handler with ExecutionChecker that reports active executions
	handler := &Handler{
		StatusStore:      store,
		RootPath:         tempDir,
		ExecutionChecker: &mockExecutionChecker{active: true},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "1.0.0",
	})

	err = handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "executions are in progress")

	// Verify the version was NOT uninstalled
	require.FileExists(t, filepath.Join(targetDir, "terraform"))
}

func TestHandleUninstall_ExecutionCheckerAllows(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup: install a version first
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "2.0.0",
		Versions: map[string]VersionStatus{
			"2.0.0": {Version: "2.0.0", State: VersionStateSucceeded},
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create the version directory
	targetDir := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "terraform"), []byte("tf"), 0o755))

	// Handler with ExecutionChecker that reports no active executions
	handler := &Handler{
		StatusStore:      store,
		RootPath:         tempDir,
		ExecutionChecker: &mockExecutionChecker{active: false},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "1.0.0",
	})

	err = handler.Handle(ctx, msg)
	require.NoError(t, err)

	// Verify the version WAS uninstalled
	require.NoFileExists(t, filepath.Join(targetDir, "terraform"))
}

func TestHandleUninstall_ExecutionCheckerError(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup: install a version first
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current: "2.0.0",
		Versions: map[string]VersionStatus{
			"2.0.0": {Version: "2.0.0", State: VersionStateSucceeded},
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create the version directory
	targetDir := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "terraform"), []byte("tf"), 0o755))

	// Handler with ExecutionChecker that returns an error
	handler := &Handler{
		StatusStore:      store,
		RootPath:         tempDir,
		ExecutionChecker: &mockExecutionChecker{err: errors.New("failed to check executions")},
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "1.0.0",
	})

	err = handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to check active executions")

	// Verify the version was NOT uninstalled due to error
	require.FileExists(t, filepath.Join(targetDir, "terraform"))
}

func TestExtractZip_SingleFileOnly(t *testing.T) {
	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "terraform")

	t.Run("single file succeeds", func(t *testing.T) {
		// Create a zip with a single file
		zipPath := filepath.Join(tempDir, "single.zip")
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		f, _ := w.Create("terraform")
		_, _ = f.Write([]byte("single binary"))
		_ = w.Close()
		require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

		err := extractZip(zipPath, targetPath)
		require.NoError(t, err)

		content, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		require.Equal(t, []byte("single binary"), content)
	})

	t.Run("multiple files rejected", func(t *testing.T) {
		// Create a zip with multiple files
		zipPath := filepath.Join(tempDir, "multi.zip")
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)

		f1, _ := w.Create("terraform")
		_, _ = f1.Write([]byte("binary1"))

		f2, _ := w.Create("malicious")
		_, _ = f2.Write([]byte("binary2"))

		_ = w.Close()
		require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

		multiTarget := filepath.Join(tempDir, "terraform-multi")
		err := extractZip(zipPath, multiTarget)
		require.Error(t, err)
		require.Contains(t, err.Error(), "multiple files")
	})

	t.Run("empty archive rejected", func(t *testing.T) {
		// Create an empty zip
		zipPath := filepath.Join(tempDir, "empty.zip")
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		_ = w.Close()
		require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

		emptyTarget := filepath.Join(tempDir, "terraform-empty")
		err := extractZip(zipPath, emptyTarget)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no file found")
	})

	t.Run("directory only archive rejected", func(t *testing.T) {
		// Create a zip with only a directory
		zipPath := filepath.Join(tempDir, "dironly.zip")
		var buf bytes.Buffer
		w := zip.NewWriter(&buf)
		_, _ = w.Create("somedir/") // Directory entry (trailing slash)
		_ = w.Close()
		require.NoError(t, os.WriteFile(zipPath, buf.Bytes(), 0o644))

		dirTarget := filepath.Join(tempDir, "terraform-dir")
		err := extractZip(zipPath, dirTarget)
		require.Error(t, err)
		require.Contains(t, err.Error(), "no file found")
	})
}

func TestStatusToResponse(t *testing.T) {
	t.Run("empty status returns not-installed state", func(t *testing.T) {
		status := &Status{}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateNotInstalled, resp.State)
		require.Empty(t, resp.CurrentVersion)
		require.Empty(t, resp.BinaryPath)
		require.Nil(t, resp.InstalledAt)
		require.Nil(t, resp.Source)
	})

	t.Run("succeeded version maps to ready state", func(t *testing.T) {
		status := &Status{
			Current: "1.6.4",
			Versions: map[string]VersionStatus{
				"1.6.4": {
					Version:   "1.6.4",
					State:     VersionStateSucceeded,
					SourceURL: "https://example.com/terraform.zip",
					Checksum:  "sha256:abc123",
				},
			},
		}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateReady, resp.State)
		require.Equal(t, "1.6.4", resp.CurrentVersion)
		require.Equal(t, "/terraform/versions/1.6.4/terraform", resp.BinaryPath)
		require.NotNil(t, resp.Source)
		require.Equal(t, "https://example.com/terraform.zip", resp.Source.URL)
		require.Equal(t, "sha256:abc123", resp.Source.Checksum)
	})

	t.Run("installing version maps to installing state", func(t *testing.T) {
		status := &Status{
			Current: "1.6.4",
			Versions: map[string]VersionStatus{
				"1.6.4": {
					Version: "1.6.4",
					State:   VersionStateInstalling,
				},
			},
		}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateInstalling, resp.State)
	})

	t.Run("failed version maps to failed state", func(t *testing.T) {
		status := &Status{
			Current: "1.6.4",
			Versions: map[string]VersionStatus{
				"1.6.4": {
					Version: "1.6.4",
					State:   VersionStateFailed,
				},
			},
		}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateFailed, resp.State)
	})

	t.Run("uninstalling version maps to uninstalling state", func(t *testing.T) {
		status := &Status{
			Current: "1.6.4",
			Versions: map[string]VersionStatus{
				"1.6.4": {
					Version: "1.6.4",
					State:   VersionStateUninstalling,
				},
			},
		}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateUninstalling, resp.State)
	})

	t.Run("uninstalled version maps to not-installed state", func(t *testing.T) {
		status := &Status{
			Current: "1.6.4",
			Versions: map[string]VersionStatus{
				"1.6.4": {
					Version: "1.6.4",
					State:   VersionStateUninstalled,
				},
			},
		}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateNotInstalled, resp.State)
	})

	t.Run("current version not in versions map returns not-installed", func(t *testing.T) {
		status := &Status{
			Current:  "1.6.4",
			Versions: map[string]VersionStatus{},
		}
		resp := status.ToResponse("/terraform")

		require.Equal(t, ResponseStateNotInstalled, resp.State)
		require.Equal(t, "1.6.4", resp.CurrentVersion) // Preserves what was set
	})

	t.Run("preserves versions map in response", func(t *testing.T) {
		status := &Status{
			Current: "1.6.4",
			Versions: map[string]VersionStatus{
				"1.5.0": {Version: "1.5.0", State: VersionStateSucceeded},
				"1.6.4": {Version: "1.6.4", State: VersionStateSucceeded},
			},
		}
		resp := status.ToResponse("/terraform")

		require.Len(t, resp.Versions, 2)
		require.Contains(t, resp.Versions, "1.5.0")
		require.Contains(t, resp.Versions, "1.6.4")
	})

	t.Run("uses tracked queue info when set", func(t *testing.T) {
		inProgress := "install:1.6.4"
		status := &Status{
			Current: "1.5.0",
			Queue: &QueueInfo{
				Pending:    2,
				InProgress: &inProgress,
			},
		}
		resp := status.ToResponse("/terraform")

		require.NotNil(t, resp.Queue)
		require.Equal(t, 2, resp.Queue.Pending)
		require.NotNil(t, resp.Queue.InProgress)
		require.Equal(t, "install:1.6.4", *resp.Queue.InProgress)
	})

	t.Run("defaults queue to empty when not set", func(t *testing.T) {
		status := &Status{
			Current: "1.5.0",
			Queue:   nil,
		}
		resp := status.ToResponse("/terraform")

		require.NotNil(t, resp.Queue)
		require.Equal(t, 0, resp.Queue.Pending)
		require.Nil(t, resp.Queue.InProgress)
	})
}

func TestGenerateVersionFromURL(t *testing.T) {
	t.Run("generates deterministic version", func(t *testing.T) {
		url := "https://example.com/terraform.zip"
		v1 := generateVersionFromURL(url)
		v2 := generateVersionFromURL(url)
		require.Equal(t, v1, v2, "same URL should generate same version")
	})

	t.Run("different URLs generate different versions", func(t *testing.T) {
		v1 := generateVersionFromURL("https://example.com/terraform1.zip")
		v2 := generateVersionFromURL("https://example.com/terraform2.zip")
		require.NotEqual(t, v1, v2, "different URLs should generate different versions")
	})

	t.Run("generated version is path-safe", func(t *testing.T) {
		v := generateVersionFromURL("https://example.com/terraform.zip")
		require.True(t, strings.HasPrefix(v, "custom-"), "version should have custom- prefix")
		require.NotContains(t, v, "/", "version should not contain path separators")
		require.NotContains(t, v, "\\", "version should not contain path separators")
		require.NotContains(t, v, "..", "version should not contain path traversal")
	})
}

func TestHandleInstall_SourceURLOnly(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	zipBytes := buildZip(t)
	sum := sha256.Sum256(zipBytes)
	checksum := "sha256:" + hex.EncodeToString(sum[:])
	sourceURL := "http://example.com/custom-terraform.zip"

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: stubTransport{body: zipBytes}},
	}

	// Install with sourceUrl only, no version
	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "", // No version specified
		SourceURL: sourceURL,
		Checksum:  checksum,
	})

	require.NoError(t, handler.Handle(ctx, msg))

	status, err := store.Get(ctx)
	require.NoError(t, err)

	// Version should be auto-generated (custom-<hash>)
	require.True(t, strings.HasPrefix(status.Current, "custom-"), "version should be auto-generated with custom- prefix")
	require.FileExists(t, filepath.Join(tempDir, "versions", status.Current, "terraform"))

	// Verify no stray empty-key entry exists
	_, hasEmptyKey := status.Versions[""]
	require.False(t, hasEmptyKey, "should not have an entry with empty version key")

	// Verify metadata is correctly preserved in the generated version entry
	vs, ok := status.Versions[status.Current]
	require.True(t, ok, "should have version entry for generated version")
	require.Equal(t, status.Current, vs.Version, "version field should match key")
	require.Equal(t, sourceURL, vs.SourceURL, "sourceURL should be preserved")
	require.Equal(t, checksum, vs.Checksum, "checksum should be preserved")
	require.Equal(t, VersionStateSucceeded, vs.State, "state should be Succeeded")
}

func TestHandleUninstall_CurrentVersionSwitchesToPrevious(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup: 2.0.0 is current, 1.0.0 is previous
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current:  "2.0.0",
		Previous: "1.0.0",
		Versions: map[string]VersionStatus{
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
			"2.0.0": {Version: "2.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create both version directories
	for _, v := range []string{"1.0.0", "2.0.0"} {
		dir := filepath.Join(tempDir, "versions", v)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terraform"), []byte("tf-"+v), 0o755))
	}

	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
	}

	// Uninstall current version (2.0.0)
	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "2.0.0",
	})

	require.NoError(t, handler.Handle(ctx, msg))

	status, err := store.Get(ctx)
	require.NoError(t, err)

	// Should have switched to previous version
	require.Equal(t, "1.0.0", status.Current, "should switch to previous version")
	require.Empty(t, status.Previous, "previous should be cleared")

	// 2.0.0 should be uninstalled
	require.Equal(t, VersionStateUninstalled, status.Versions["2.0.0"].State)
	require.NoFileExists(t, filepath.Join(tempDir, "versions", "2.0.0", "terraform"))

	// Symlink should point to 1.0.0
	symlinkPath := filepath.Join(tempDir, "current")
	target, err := os.Readlink(symlinkPath)
	require.NoError(t, err)
	require.Contains(t, target, "1.0.0")
}

func TestHandleUninstall_CurrentVersionNoPrevious(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup: only 1.0.0 is installed (no previous)
	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	err := store.Put(ctx, &Status{
		Current:  "1.0.0",
		Previous: "",
		Versions: map[string]VersionStatus{
			"1.0.0": {Version: "1.0.0", State: VersionStateSucceeded},
		},
	})
	require.NoError(t, err)

	// Create version directory
	dir := filepath.Join(tempDir, "versions", "1.0.0")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "terraform"), []byte("tf"), 0o755))

	// Create current symlink
	symlinkPath := filepath.Join(tempDir, "current")
	require.NoError(t, os.Symlink(filepath.Join(dir, "terraform"), symlinkPath))

	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
	}

	// Uninstall current version (1.0.0)
	msg := queue.NewMessage(JobMessage{
		Operation: OperationUninstall,
		Version:   "1.0.0",
	})

	require.NoError(t, handler.Handle(ctx, msg))

	status, err := store.Get(ctx)
	require.NoError(t, err)

	// Current should be cleared
	require.Empty(t, status.Current, "current should be cleared")
	require.Empty(t, status.Previous, "previous should remain empty")

	// 1.0.0 should be uninstalled
	require.Equal(t, VersionStateUninstalled, status.Versions["1.0.0"].State)
	require.NoFileExists(t, filepath.Join(tempDir, "versions", "1.0.0", "terraform"))

	// Symlink should be removed
	_, err = os.Lstat(symlinkPath)
	require.True(t, os.IsNotExist(err), "symlink should be removed")
}
