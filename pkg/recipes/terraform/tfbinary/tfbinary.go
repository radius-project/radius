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

// Package tfbinary downloads and places the shared Terraform binary used by Terraform recipes.
// It has minimal dependencies so that the small pre-upgrade image can use it to install Terraform
// from an init container, keeping the download out of the resource provider containers.
package tfbinary

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/hashicorp/hc-install/product"
	"github.com/hashicorp/hc-install/releases"
	"github.com/hashicorp/hc-install/src"
)

const (
	// DefaultGlobalDir is the shared directory the resource providers read the Terraform binary from.
	DefaultGlobalDir = "/terraform/.terraform-global"

	// BinaryName is the file name of the Terraform binary in the global directory.
	BinaryName = "terraform"

	// MarkerFileName is written to the global directory once the binary has been installed and verified.
	MarkerFileName = ".terraform-ready"

	// maxArchiveBytes and maxBinaryBytes bound a custom download so a bad or malicious URL
	// cannot exhaust the scratch volume. Official Terraform archives are well under these limits.
	maxArchiveBytes = 512 << 20
	maxBinaryBytes  = 1 << 30
)

// Download downloads the given Terraform version and places the binary at dst with mode 0755.
//
// hc-install verifies the release checksum and signature and unpacks into a temporary directory.
// The binary is then renamed into place, so a process running an existing binary at dst is never
// affected and readers never observe a partially written file.
func Download(ctx context.Context, installer *install.Installer, v *version.Version, dst string) (err error) {
	execPath, err := installer.Ensure(ctx, []src.Source{
		&releases.ExactVersion{Product: product.Terraform, Version: v},
	})
	if err != nil {
		return fmt.Errorf("failed to download Terraform %s: %w", v, err)
	}
	defer func() {
		if removeErr := installer.Remove(ctx); removeErr != nil && err == nil {
			err = fmt.Errorf("failed to remove temporary Terraform download: %w", removeErr)
		}
	}()

	if err := CopyExecutable(execPath, dst); err != nil {
		return fmt.Errorf("failed to copy Terraform binary to %s: %w", dst, err)
	}
	return nil
}

// DownloadURL downloads a Terraform release zip from url and places the terraform binary at dst
// with mode 0755. It is used for custom mirrors, which hc-install does not support.
//
// When sha256Hex is set, the archive must match it. The archive is staged in os.TempDir and the
// binary is renamed into place, so readers never observe a partially written file.
func DownloadURL(ctx context.Context, client *http.Client, url, sha256Hex, dst string) error {
	archive, err := os.CreateTemp("", "terraform-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(archive.Name())
	defer archive.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("invalid Terraform download URL: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download Terraform: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download Terraform: unexpected status %s", resp.Status)
	}

	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, hash), io.LimitReader(resp.Body, maxArchiveBytes+1))
	if err != nil {
		return fmt.Errorf("failed to download Terraform: %w", err)
	}
	if n > maxArchiveBytes {
		return fmt.Errorf("terraform archive exceeds %d bytes", maxArchiveBytes)
	}

	if sha256Hex != "" {
		if got := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(got, sha256Hex) {
			return fmt.Errorf("terraform archive checksum mismatch: expected sha256 %s, got %s", strings.ToLower(sha256Hex), got)
		}
	}

	return extractBinary(archive, n, dst)
}

// extractBinary copies the top-level terraform entry from the zip archive to dst.
func extractBinary(archive io.ReaderAt, size int64, dst string) error {
	zr, err := zip.NewReader(archive, size)
	if err != nil {
		return fmt.Errorf("failed to read Terraform archive: %w", err)
	}

	for _, f := range zr.File {
		if f.Name != BinaryName {
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("terraform entry in archive is not a regular file")
		}
		if f.UncompressedSize64 > maxBinaryBytes {
			return fmt.Errorf("terraform binary in archive exceeds %d bytes", maxBinaryBytes)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to read terraform from archive: %w", err)
		}
		defer rc.Close()
		if err := writeExecutable(io.LimitReader(rc, maxBinaryBytes), dst); err != nil {
			return fmt.Errorf("failed to install terraform binary to %s: %w", dst, err)
		}
		return nil
	}
	return errors.New("terraform archive does not contain a top-level terraform binary")
}

// CopyExecutable streams src to dst with mode 0755 without buffering the whole binary in memory.
// The copy is written to a temporary file in dst's directory and renamed into place.
func CopyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	return writeExecutable(in, dst)
}

// writeExecutable writes r to a temporary file next to dst with mode 0755 and renames it into place.
func writeExecutable(r io.Reader, dst string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), dst)
}
