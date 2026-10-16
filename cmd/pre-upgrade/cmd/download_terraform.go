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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/go-logr/logr"
	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"
	"github.com/spf13/cobra"

	"github.com/radius-project/radius/pkg/recipes/terraform/tfbinary"
)

const (
	defaultTerraformDownloadTimeout = 5 * time.Minute
	terraformVerifyTimeout          = 30 * time.Second

	// legacyTerraformSourceFile and legacyTerraformSource match what the previous shell-based
	// terraform-init container wrote to rp.terraform.path.
	legacyTerraformSourceFile = ".terraform-source"
	legacyTerraformSource     = "pre-mounted"
)

type downloadTerraformOptions struct {
	version   string
	dir       string
	legacyDir string
	timeout   time.Duration
}

// terraformDownloader downloads the given Terraform version to dst.
type terraformDownloader func(ctx context.Context, v *version.Version, dst string) error

// terraformVerifier checks that the binary at path runs.
type terraformVerifier func(ctx context.Context, path string) error

func newDownloadTerraformCommand() *cobra.Command {
	options := downloadTerraformOptions{
		dir:     tfbinary.DefaultGlobalDir,
		timeout: defaultTerraformDownloadTimeout,
	}

	cmd := &cobra.Command{
		Use:   "download-terraform",
		Short: "Download and verify the Terraform binary shared with the Radius resource providers",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			v, err := options.validate()
			if err != nil {
				return err
			}

			logger, flush, err := newLogger()
			if err != nil {
				return fmt.Errorf("failed to initialize logger: %w", err)
			}
			defer flush()

			ctx, cancel := context.WithTimeout(cmd.Context(), options.timeout)
			defer cancel()

			return downloadTerraform(ctx, options, v, logger, hcInstallDownloader, runTerraformVersion)
		},
	}

	cmd.Flags().StringVar(&options.version, "version", "", "Terraform version to download (required)")
	cmd.Flags().StringVar(&options.dir, "dir", options.dir, "Directory to install the Terraform binary and ready marker into")
	cmd.Flags().StringVar(&options.legacyDir, "legacy-dir", "", "Optional directory that also receives a copy of the binary and a .terraform-source marker")
	cmd.Flags().DurationVar(&options.timeout, "timeout", options.timeout, "Maximum time to download and verify Terraform")
	return cmd
}

func (o downloadTerraformOptions) validate() (*version.Version, error) {
	switch {
	case o.version == "":
		return nil, fmt.Errorf("version is required")
	case o.dir == "":
		return nil, fmt.Errorf("dir is required")
	case o.timeout <= 0:
		return nil, fmt.Errorf("timeout must be greater than zero")
	}

	v, err := version.NewVersion(o.version)
	if err != nil {
		return nil, fmt.Errorf("invalid Terraform version %q: %w", o.version, err)
	}
	return v, nil
}

func downloadTerraform(ctx context.Context, o downloadTerraformOptions, v *version.Version, logger logr.Logger, download terraformDownloader, verify terraformVerifier) error {
	if err := os.MkdirAll(o.dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", o.dir, err)
	}

	binary := filepath.Join(o.dir, tfbinary.BinaryName)
	start := time.Now()
	logger.Info("Downloading Terraform", "terraformVersion", v.String(), "path", binary)
	if err := download(ctx, v, binary); err != nil {
		return err
	}
	logger.Info("Downloaded Terraform", "duration", time.Since(start).String())

	if err := verify(ctx, binary); err != nil {
		return fmt.Errorf("failed to verify Terraform binary: %w", err)
	}

	if o.legacyDir != "" {
		if err := tfbinary.CopyExecutable(binary, filepath.Join(o.legacyDir, tfbinary.BinaryName)); err != nil {
			return fmt.Errorf("failed to copy Terraform binary to %s: %w", o.legacyDir, err)
		}
		if err := os.WriteFile(filepath.Join(o.legacyDir, legacyTerraformSourceFile), []byte(legacyTerraformSource+"\n"), 0644); err != nil {
			return fmt.Errorf("failed to write legacy Terraform marker: %w", err)
		}
	}

	// The ready marker is written last so the resource providers only use a verified binary.
	if err := os.WriteFile(filepath.Join(o.dir, tfbinary.MarkerFileName), nil, 0644); err != nil {
		return fmt.Errorf("failed to write Terraform ready marker: %w", err)
	}

	logger.Info("Terraform is ready", "terraformVersion", v.String(), "path", binary)
	return nil
}

func hcInstallDownloader(ctx context.Context, v *version.Version, dst string) error {
	return tfbinary.Download(ctx, install.NewInstaller(), v, dst)
}

func runTerraformVersion(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, terraformVerifyTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}
