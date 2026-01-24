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

package installer

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestHandleInstall_ExistingLockFileFailsBusy(t *testing.T) {
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
	// Error can be "path separator" or "path traversal" depending on which check catches it first
	require.True(t, strings.Contains(err.Error(), "path separator") || strings.Contains(err.Error(), "path traversal"),
		"expected error to contain 'path separator' or 'path traversal', got: %s", err.Error())

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
	// Error can be "path separator" or "path traversal" depending on which check catches it first
	require.True(t, strings.Contains(err.Error(), "path separator") || strings.Contains(err.Error(), "path traversal"),
		"expected error to contain 'path separator' or 'path traversal', got: %s", err.Error())
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

// validTestCACert is a self-signed CA certificate for testing purposes.
// Generated specifically for unit tests - not for production use.
// NOTE: This certificate expires on 2027-01-21. If tests start failing after
// that date, generate a new certificate with a longer validity period:
//   openssl req -x509 -newkey rsa:2048 -keyout /dev/null -out ca.pem -days 3650 -nodes -subj "/CN=testca"
// Then replace the certificate below with the contents of ca.pem.
const validTestCACert = `-----BEGIN CERTIFICATE-----
MIIDAzCCAeugAwIBAgIUM06Yo/BKCPvBfZwztaJPszhAO98wDQYJKoZIhvcNAQEL
BQAwETEPMA0GA1UEAwwGdGVzdGNhMB4XDTI2MDEyMTEwMjAzNVoXDTI3MDEyMTEw
MjAzNVowETEPMA0GA1UEAwwGdGVzdGNhMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA0wyOmcNaSz1AQHGNVmNzzkDO5VhUCv56KRybhLR/uXhapxQ4T+Rr
beMUExEaxyWDnTjsnirNUvwadBONWzm8cDQSW2KldbnzjteBRlNDbRI6TgKE0TRR
ljAM77Dczzuye2PsQS002Ny3UR+MnzI1kA3/XjAeAVefKn31Col0Ssn7OdvZ1VTH
aK04b2szaAla5Sl+eWKUsxj6UA/V/Xq94Z4AEnqk7zkGxnpILvxcz0QY/U/7e5iQ
IM/NkIeMoJe+Cfij+yPqLgh2f5L4Vi9WvRB8P0rbvl5WrEU6K6bjuZ5zKxiC+rbU
5hjAlR5lyrgo8cwiB5cOah+qQzl/3c26yQIDAQABo1MwUTAdBgNVHQ4EFgQU8/CI
UhXWPvHMCIynxKS4D+PQdy0wHwYDVR0jBBgwFoAU8/CIUhXWPvHMCIynxKS4D+PQ
dy0wDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAevFg7NV4D6UP
qYdvGjWgMFEUiUBp5EtEU5KD7FZwKop/lFqnvo+L1bUUy2hab76eO+g0perp8b8j
/ZwMgdIVNjNEWgM8h+Gg3HG8Rvdle5NqMq4lIGzmTN+MhPnQ8rECMSm0nVGTtFA0
qE+O0LoSl/4FL9pUQuwZi+WibxoTOlw3NXpxx2WUFzU/Giwx6OYCTb773M9noKCH
7VAkvFImjSbr4SU05DGe+cUcWmtWcfhj2geiCHl/EEpe/oEi5/XnpgeMj4vkE6zK
fiCLJ0WJ77/ohDKnNecDZKIWLsUo9ywMJqi9TLSiBf5oMOc9uZtDoPTPzsXzcPZP
2JkLUbkliQ==
-----END CERTIFICATE-----`

func TestCreateTLSClient(t *testing.T) {
	t.Run("valid CA bundle creates client successfully", func(t *testing.T) {
		client, err := createTLSClient(&tlsClientOptions{CABundle: validTestCACert})
		require.NoError(t, err)
		require.NotNil(t, client)
		require.NotNil(t, client.Transport)

		transport, ok := client.Transport.(*http.Transport)
		require.True(t, ok, "transport should be *http.Transport")
		require.NotNil(t, transport.TLSClientConfig)
		require.NotNil(t, transport.TLSClientConfig.RootCAs)
	})

	t.Run("invalid CA bundle returns error", func(t *testing.T) {
		invalidCert := "not a valid PEM certificate"
		client, err := createTLSClient(&tlsClientOptions{CABundle: invalidCert})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "failed to parse CA bundle")
	})

	t.Run("empty CA bundle creates client without custom RootCAs", func(t *testing.T) {
		client, err := createTLSClient(&tlsClientOptions{CABundle: ""})
		require.NoError(t, err)
		require.NotNil(t, client)
		transport := client.Transport.(*http.Transport)
		require.Nil(t, transport.TLSClientConfig.RootCAs, "RootCAs should be nil when no CA bundle is provided")
	})

	t.Run("CA bundle with only whitespace returns error", func(t *testing.T) {
		client, err := createTLSClient(&tlsClientOptions{CABundle: "   \n\t  "})
		require.Error(t, err)
		require.Nil(t, client)
		require.Contains(t, err.Error(), "failed to parse CA bundle")
	})

	t.Run("malformed PEM returns error", func(t *testing.T) {
		malformedPEM := `-----BEGIN CERTIFICATE-----
not-valid-base64-content
-----END CERTIFICATE-----`
		client, err := createTLSClient(&tlsClientOptions{CABundle: malformedPEM})
		require.Error(t, err)
		require.Nil(t, client)
	})

	t.Run("TLS config has minimum version TLS 1.2", func(t *testing.T) {
		client, err := createTLSClient(&tlsClientOptions{CABundle: validTestCACert})
		require.NoError(t, err)
		require.NotNil(t, client)

		transport := client.Transport.(*http.Transport)
		require.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion, "MinVersion should be TLS 1.2")
	})
}

func TestHandleInstall_WithCABundle(t *testing.T) {
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

	// Install with CA bundle - note that with a custom HTTPClient, the CA bundle
	// won't be used (HTTPClient takes precedence), but it should still be stored in job
	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "https://example.com/terraform.zip",
		Checksum:  checksum,
		CABundle:  validTestCACert,
	})

	require.NoError(t, handler.Handle(ctx, msg))

	status, err := store.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", status.Current)
	vs := status.Versions["1.0.0"]
	require.Equal(t, VersionStateSucceeded, vs.State)
}

func TestHandleInstall_InvalidCABundleFails(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	zipBytes := buildZip(t)
	sum := sha256.Sum256(zipBytes)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	// No HTTPClient - will try to use CA bundle
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
	}

	msg := queue.NewMessage(JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "https://example.com/terraform.zip",
		Checksum:  checksum,
		CABundle:  "invalid-ca-bundle",
	})

	err := handler.Handle(ctx, msg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to configure HTTP client")

	// Verify the version was marked as failed
	status, _ := store.Get(ctx)
	vs := status.Versions["1.0.0"]
	require.Equal(t, VersionStateFailed, vs.State)
}

func TestDownload_UsesCABundleWhenNoHTTPClient(t *testing.T) {
	// This test verifies the download function creates a custom client when CABundle is provided
	// We can't easily test actual TLS behavior without a real server, but we can verify the code path
	ctx := context.Background()
	tempDir := t.TempDir()

	handler := &Handler{
		RootPath:   tempDir,
		HTTPClient: nil, // No default client - will create one with CA bundle
	}

	// Create a test file to download "from"
	// This will fail because we're not actually serving HTTPS, but we can verify the error
	dstPath := filepath.Join(tempDir, "download")

	// Test with invalid CA bundle - should fail at CA parsing
	err := handler.download(ctx, &downloadOptions{
		URL:      "https://localhost:9999/nonexistent",
		Dst:      dstPath,
		CABundle: "invalid-pem",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to configure HTTP client")
}

func TestDownload_NoCABundleUsesDefaultClient(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	// Setup a test server
	content := []byte("test content")
	server := setupTestServer(t, content)

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  nil, // Will use default client
	}

	dstPath := filepath.Join(tempDir, "download")
	err := handler.download(ctx, &downloadOptions{
		URL: server.URL + "/terraform",
		Dst: dstPath,
	})
	require.NoError(t, err)

	// Verify file was downloaded
	downloaded, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	require.Equal(t, content, downloaded)
}

func setupTestServer(t *testing.T, content []byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
}

func TestJobMessage_CABundleSerialization(t *testing.T) {
	// Test that CABundle is properly serialized/deserialized in JobMessage
	original := JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "https://example.com/terraform.zip",
		Checksum:  "sha256:abc123",
		CABundle:  validTestCACert,
	}

	// Serialize
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Verify CABundle is included in JSON
	require.Contains(t, string(data), "caBundle")
	require.Contains(t, string(data), "BEGIN CERTIFICATE")

	// Deserialize
	var decoded JobMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, original.Operation, decoded.Operation)
	require.Equal(t, original.Version, decoded.Version)
	require.Equal(t, original.SourceURL, decoded.SourceURL)
	require.Equal(t, original.Checksum, decoded.Checksum)
	require.Equal(t, original.CABundle, decoded.CABundle)
}

func TestJobMessage_CABundleOmittedWhenEmpty(t *testing.T) {
	// Test that CABundle is omitted from JSON when empty (omitempty)
	msg := JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		CABundle:  "",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	// Should not contain caBundle key when empty
	require.NotContains(t, string(data), "caBundle")
}

// Additional CA Bundle Edge Case Tests

func TestCreateTLSClient_MultipleCertificates(t *testing.T) {
	// Test that multiple certificates in a single bundle are all parsed
	multipleCerts := validTestCACert + "\n" + validTestCACert
	client, err := createTLSClient(&tlsClientOptions{CABundle: multipleCerts})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestCreateTLSClient_CertWithLeadingWhitespace(t *testing.T) {
	// Test CA bundle with leading newlines (common in copy-paste scenarios)
	// Note: Leading spaces before "-----BEGIN" will cause parsing to fail
	// because PEM decoder looks for "-----BEGIN" at line start
	certWithWhitespace := "\n\n" + validTestCACert
	client, err := createTLSClient(&tlsClientOptions{CABundle: certWithWhitespace})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestCreateTLSClient_CertWithTrailingWhitespace(t *testing.T) {
	// Test CA bundle with trailing whitespace
	certWithWhitespace := validTestCACert + "\n\n  "
	client, err := createTLSClient(&tlsClientOptions{CABundle: certWithWhitespace})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestCreateTLSClient_CertWithWindowsLineEndings(t *testing.T) {
	// Test CA bundle with Windows-style line endings (CRLF)
	certWithCRLF := strings.ReplaceAll(validTestCACert, "\n", "\r\n")
	client, err := createTLSClient(&tlsClientOptions{CABundle: certWithCRLF})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestCreateTLSClient_PartiallyValidBundle(t *testing.T) {
	// Test bundle where first cert is invalid but second is valid
	// AppendCertsFromPEM skips invalid certs and returns true if at least one was added
	mixedBundle := "not a cert\n" + validTestCACert
	client, err := createTLSClient(&tlsClientOptions{CABundle: mixedBundle})
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestCreateTLSClient_OnlyInvalidCerts(t *testing.T) {
	// Test bundle where all certs are invalid
	invalidBundle := `-----BEGIN CERTIFICATE-----
invalid-base64-content
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
also-invalid
-----END CERTIFICATE-----`
	client, err := createTLSClient(&tlsClientOptions{CABundle: invalidBundle})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "failed to parse CA bundle")
}

func TestDownload_HTTPClientTakesPrecedenceOverCABundle(t *testing.T) {
	// Test that when HTTPClient is set, it takes precedence over CA bundle
	ctx := context.Background()
	tempDir := t.TempDir()

	content := []byte("test binary content")

	customClientCalled := false
	customClient := &http.Client{
		Transport: &trackingTransport{
			onRequest: func() { customClientCalled = true },
			body:      content,
		},
	}

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  customClient, // Custom client set
	}

	dstPath := filepath.Join(tempDir, "download")
	// Even though CA bundle is provided, custom HTTPClient should be used.
	// The trackingTransport intercepts the request directly, so no real HTTP call is made.
	err := handler.download(ctx, &downloadOptions{
		URL:      "https://example.com/terraform",
		Dst:      dstPath,
		CABundle: validTestCACert,
	})
	require.NoError(t, err)
	require.True(t, customClientCalled, "custom HTTPClient should be used when set")
}

func TestInstallRequest_CABundleSerialization(t *testing.T) {
	// Test that InstallRequest correctly serializes/deserializes CABundle
	req := InstallRequest{
		Version:   "1.6.4",
		SourceURL: "https://example.com/terraform.zip",
		Checksum:  "sha256:abc123",
		CABundle:  validTestCACert,
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Verify all fields are present
	require.Contains(t, string(data), `"version":"1.6.4"`)
	require.Contains(t, string(data), `"sourceUrl":"https://example.com/terraform.zip"`)
	require.Contains(t, string(data), `"checksum":"sha256:abc123"`)
	require.Contains(t, string(data), `"caBundle"`)

	// Deserialize and verify
	var decoded InstallRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	require.Equal(t, req.Version, decoded.Version)
	require.Equal(t, req.SourceURL, decoded.SourceURL)
	require.Equal(t, req.Checksum, decoded.Checksum)
	require.Equal(t, req.CABundle, decoded.CABundle)
}

func TestInstallRequest_CABundleOmittedWhenEmpty(t *testing.T) {
	// Test that CABundle is omitted from JSON when empty
	req := InstallRequest{
		Version:   "1.6.4",
		SourceURL: "https://example.com/terraform.zip",
		CABundle:  "",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Should not contain caBundle key when empty
	require.NotContains(t, string(data), "caBundle")
}

func TestHandleInstall_CABundlePassedThroughJobMessage(t *testing.T) {
	// Verify that CA bundle is correctly passed through the job message
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

	// Create job message with CA bundle
	jobMsg := JobMessage{
		Operation: OperationInstall,
		Version:   "1.0.0",
		SourceURL: "https://example.com/terraform.zip",
		Checksum:  checksum,
		CABundle:  validTestCACert,
	}

	// Serialize and deserialize to simulate queue transport
	data, err := json.Marshal(jobMsg)
	require.NoError(t, err)

	var decodedJob JobMessage
	err = json.Unmarshal(data, &decodedJob)
	require.NoError(t, err)

	// Verify CA bundle survived serialization
	require.Equal(t, validTestCACert, decodedJob.CABundle)

	// Create queue message with the job
	msg := queue.NewMessage(decodedJob)

	// Handle the message
	err = handler.Handle(ctx, msg)
	require.NoError(t, err)

	// Verify installation succeeded
	status, err := store.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", status.Current)
}

// Tests for Auth Header support

func TestDownload_WithAuthHeader(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	content := []byte("test binary")
	var receivedAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	handler := &Handler{
		RootPath: tempDir,
	}

	dstPath := filepath.Join(tempDir, "download")
	err := handler.download(ctx, &downloadOptions{
		URL:        server.URL + "/terraform",
		Dst:        dstPath,
		AuthHeader: "Bearer test-token-123",
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer test-token-123", receivedAuthHeader)

	downloaded, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	require.Equal(t, content, downloaded)
}

func TestDownload_WithBasicAuth(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	content := []byte("test binary")
	var receivedAuthHeader string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer server.Close()

	handler := &Handler{
		RootPath: tempDir,
	}

	dstPath := filepath.Join(tempDir, "download")
	err := handler.download(ctx, &downloadOptions{
		URL:        server.URL + "/terraform",
		Dst:        dstPath,
		AuthHeader: "Basic dXNlcjpwYXNz", // base64("user:pass")
	})
	require.NoError(t, err)
	require.Equal(t, "Basic dXNlcjpwYXNz", receivedAuthHeader)
}

func TestHandleInstall_WithAuthHeader(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	zipBytes := buildZip(t)
	sum := sha256.Sum256(zipBytes)
	checksum := "sha256:" + hex.EncodeToString(sum[:])

	var receivedAuthHeader string
	transport := &authCapturingTransport{
		body:       zipBytes,
		captureAuth: func(auth string) { receivedAuthHeader = auth },
	}

	store := NewStatusStore(inmemory.NewClient(), StatusStorageID)
	handler := &Handler{
		StatusStore: store,
		RootPath:    tempDir,
		HTTPClient:  &http.Client{Transport: transport},
	}

	msg := queue.NewMessage(JobMessage{
		Operation:  OperationInstall,
		Version:    "1.0.0",
		SourceURL:  "https://example.com/terraform.zip",
		Checksum:   checksum,
		AuthHeader: "Bearer my-token",
	})

	require.NoError(t, handler.Handle(ctx, msg))
	require.Equal(t, "Bearer my-token", receivedAuthHeader)

	status, err := store.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "1.0.0", status.Current)
}

// authCapturingTransport captures the Authorization header
type authCapturingTransport struct {
	body        []byte
	captureAuth func(string)
}

func (t *authCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.captureAuth != nil {
		t.captureAuth(req.Header.Get("Authorization"))
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(t.body)),
		Header:     make(http.Header),
	}, nil
}

// Tests for mTLS (Client Certificate) support

// validTestClientCert is a valid self-signed EC certificate for testing mTLS
const validTestClientCert = `-----BEGIN CERTIFICATE-----
MIIBgDCCASegAwIBAgIUO66xXGDU8mbkBLlWDIedYMe36KQwCgYIKoZIzj0EAwIw
FjEUMBIGA1UEAwwLdGVzdC1jbGllbnQwHhcNMjYwMTIxMTEwOTU1WhcNMjcwMTIx
MTEwOTU1WjAWMRQwEgYDVQQDDAt0ZXN0LWNsaWVudDBZMBMGByqGSM49AgEGCCqG
SM49AwEHA0IABPYLQEfPKg1q93kkfzMq3mmCjPQ4n67c5ZTvy2KZp0SkudA87onK
Uc0kaAlkWYP9en/guhBPEIymeP7FDXMRi3+jUzBRMB0GA1UdDgQWBBT7fcIawlf7
eDhdmCnVc0pWvocf/jAfBgNVHSMEGDAWgBT7fcIawlf7eDhdmCnVc0pWvocf/jAP
BgNVHRMBAf8EBTADAQH/MAoGCCqGSM49BAMCA0cAMEQCIDYmsM0xMvcCUTwKHSNZ
9fIQUuA3sE0lwiMKTJjxVaXgAiAqvAlZYNOO9hm3SRzum4X1k5esFZk/rA9DsP96
OUSd/A==
-----END CERTIFICATE-----`

// validTestClientKey is the private key corresponding to validTestClientCert
const validTestClientKey = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgMHOcZaCPvsej89Um
UEvIdBzlodyitFxw8a51JBJat7WhRANCAAT2C0BHzyoNavd5JH8zKt5pgoz0OJ+u
3OWU78timadEpLnQPO6JylHNJGgJZFmD/Xp/4LoQTxCMpnj+xQ1zEYt/
-----END PRIVATE KEY-----`

func TestCreateTLSClient_WithClientCert(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		ClientCert: validTestClientCert,
		ClientKey:  validTestClientKey,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport := client.Transport.(*http.Transport)
	require.NotNil(t, transport.TLSClientConfig)
	require.Len(t, transport.TLSClientConfig.Certificates, 1, "should have one client certificate")
}

func TestCreateTLSClient_ClientCertWithoutKey(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		ClientCert: validTestClientCert,
		// ClientKey intentionally missing
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "both client certificate and key must be provided")
}

func TestCreateTLSClient_ClientKeyWithoutCert(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		// ClientCert intentionally missing
		ClientKey: validTestClientKey,
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "both client certificate and key must be provided")
}

func TestCreateTLSClient_InvalidClientCert(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		ClientCert: "not a valid certificate",
		ClientKey:  "not a valid key",
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "failed to load client certificate")
}

func TestCreateTLSClient_WithCABundleAndClientCert(t *testing.T) {
	// Test that both CA bundle and client cert can be used together
	client, err := createTLSClient(&tlsClientOptions{
		CABundle:   validTestCACert,
		ClientCert: validTestClientCert,
		ClientKey:  validTestClientKey,
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport := client.Transport.(*http.Transport)
	require.NotNil(t, transport.TLSClientConfig.RootCAs)
	require.Len(t, transport.TLSClientConfig.Certificates, 1)
}

// Tests for Proxy support

func TestCreateTLSClient_WithProxy(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		ProxyURL: "http://proxy.example.com:8080",
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport := client.Transport.(*http.Transport)
	require.NotNil(t, transport.Proxy, "Proxy should be configured")
}

func TestCreateTLSClient_WithInvalidProxyURL(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		ProxyURL: "not-a-valid-url",
	})
	require.Error(t, err)
	require.Nil(t, client)
	require.Contains(t, err.Error(), "proxy URL must use http or https scheme")
}

func TestCreateTLSClient_WithProxyMissingScheme(t *testing.T) {
	client, err := createTLSClient(&tlsClientOptions{
		ProxyURL: "proxy.example.com:8080",
	})
	require.Error(t, err)
	require.Nil(t, client)
}

func TestCreateTLSClient_AllOptions(t *testing.T) {
	// Test with all options configured
	client, err := createTLSClient(&tlsClientOptions{
		CABundle:   validTestCACert,
		ClientCert: validTestClientCert,
		ClientKey:  validTestClientKey,
		ProxyURL:   "http://proxy.example.com:8080",
	})
	require.NoError(t, err)
	require.NotNil(t, client)

	transport := client.Transport.(*http.Transport)
	require.NotNil(t, transport.TLSClientConfig.RootCAs)
	require.Len(t, transport.TLSClientConfig.Certificates, 1)
	require.NotNil(t, transport.Proxy)
}

// Tests for JobMessage and InstallRequest serialization with new fields

func TestJobMessage_NewFieldsSerialization(t *testing.T) {
	original := JobMessage{
		Operation:  OperationInstall,
		Version:    "1.0.0",
		SourceURL:  "https://example.com/terraform.zip",
		Checksum:   "sha256:abc123",
		CABundle:   validTestCACert,
		AuthHeader: "Bearer token123",
		ClientCert: "cert-data",
		ClientKey:  "key-data",
		ProxyURL:   "http://proxy:8080",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded JobMessage
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, original.Operation, decoded.Operation)
	require.Equal(t, original.Version, decoded.Version)
	require.Equal(t, original.SourceURL, decoded.SourceURL)
	require.Equal(t, original.Checksum, decoded.Checksum)
	require.Equal(t, original.CABundle, decoded.CABundle)
	require.Equal(t, original.AuthHeader, decoded.AuthHeader)
	require.Equal(t, original.ClientCert, decoded.ClientCert)
	require.Equal(t, original.ClientKey, decoded.ClientKey)
	require.Equal(t, original.ProxyURL, decoded.ProxyURL)
}

func TestInstallRequest_NewFieldsSerialization(t *testing.T) {
	original := InstallRequest{
		Version:    "1.6.4",
		SourceURL:  "https://example.com/terraform.zip",
		Checksum:   "sha256:abc123",
		CABundle:   validTestCACert,
		AuthHeader: "Basic dXNlcjpwYXNz",
		ClientCert: "cert-pem-data",
		ClientKey:  "key-pem-data",
		ProxyURL:   "https://proxy.corp.com:8080",
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded InstallRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	require.Equal(t, original.Version, decoded.Version)
	require.Equal(t, original.SourceURL, decoded.SourceURL)
	require.Equal(t, original.Checksum, decoded.Checksum)
	require.Equal(t, original.CABundle, decoded.CABundle)
	require.Equal(t, original.AuthHeader, decoded.AuthHeader)
	require.Equal(t, original.ClientCert, decoded.ClientCert)
	require.Equal(t, original.ClientKey, decoded.ClientKey)
	require.Equal(t, original.ProxyURL, decoded.ProxyURL)
}

func TestParseProxyURL(t *testing.T) {
	tests := []struct {
		name      string
		proxyURL  string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid http proxy",
			proxyURL:  "http://proxy.example.com:8080",
			expectErr: false,
		},
		{
			name:      "valid https proxy",
			proxyURL:  "https://proxy.example.com:8443",
			expectErr: false,
		},
		{
			name:      "proxy with auth",
			proxyURL:  "http://user:pass@proxy.example.com:8080",
			expectErr: false,
		},
		{
			name:      "invalid scheme",
			proxyURL:  "ftp://proxy.example.com:8080",
			expectErr: true,
			errMsg:    "proxy URL must use http or https scheme",
		},
		{
			name:      "missing host",
			proxyURL:  "http://",
			expectErr: true,
			errMsg:    "proxy URL must have a host",
		},
		{
			name:      "no scheme",
			proxyURL:  "proxy.example.com:8080",
			expectErr: true,
			errMsg:    "proxy URL must use http or https scheme",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := parseProxyURL(tc.proxyURL)
			if tc.expectErr {
				require.Error(t, err)
				if tc.errMsg != "" {
					require.Contains(t, err.Error(), tc.errMsg)
				}
				require.Nil(t, parsed)
			} else {
				require.NoError(t, err)
				require.NotNil(t, parsed)
			}
		})
	}
}
