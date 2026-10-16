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
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// CopyExecutable streams src to dst with mode 0755 without buffering the whole binary in memory.
// The copy is written to a temporary file in dst's directory and renamed into place.
func CopyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, in); err != nil {
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
