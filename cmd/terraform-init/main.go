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

// terraform-init downloads the Terraform binary that the Radius resource providers use to run
// Terraform recipes. The Helm chart runs it as an init container in the applications-rp and
// dynamic-rp pods, so the download happens outside the resource provider containers and a failed
// download blocks the pod. It is an internal component and is not intended to be run by users.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hashicorp/go-version"
	install "github.com/hashicorp/hc-install"

	"github.com/radius-project/radius/pkg/recipes/terraform/tfbinary"
)

const (
	defaultDownloadTimeout = 5 * time.Minute
	verifyTimeout          = 30 * time.Second

	// legacySourceFile and legacySource match what the previous shell-based terraform-init
	// container wrote to rp.terraform.path.
	legacySourceFile = ".terraform-source"
	legacySource     = "pre-mounted"
)

type options struct {
	version   string
	url       string
	sha256    string
	dir       string
	legacyDir string
	timeout   time.Duration
}

// downloader downloads Terraform to dst.
type downloader func(ctx context.Context, dst string) error

// verifier runs the binary at path and returns the Terraform version it reports.
type verifier func(ctx context.Context, path string) (string, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(ctx, os.Args[1:], logger); err != nil {
		logger.Error("Terraform init failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, logger *slog.Logger) error {
	o, err := parseFlags(args)
	if err != nil {
		return err
	}
	want, err := o.validate()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	var download downloader
	if want != nil {
		download = func(ctx context.Context, dst string) error {
			return tfbinary.Download(ctx, install.NewInstaller(), want, dst)
		}
	} else {
		if o.sha256 == "" {
			logger.Warn("No checksum was provided for the custom Terraform download URL, so the archive is not verified. Set global.terraform.downloadChecksum to verify it.")
		}
		download = func(ctx context.Context, dst string) error {
			return tfbinary.DownloadURL(ctx, http.DefaultClient, o.url, o.sha256, dst)
		}
	}

	return installTerraform(ctx, o, want, logger, download, terraformVersion)
}

func parseFlags(args []string) (options, error) {
	o := options{dir: tfbinary.DefaultGlobalDir, timeout: defaultDownloadTimeout}

	fs := flag.NewFlagSet("terraform-init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.version, "version", "", "Official Terraform release to download. Mutually exclusive with --url.")
	fs.StringVar(&o.url, "url", "", "URL of a Terraform release zip to download instead of an official release.")
	fs.StringVar(&o.sha256, "sha256", "", "Expected SHA-256 checksum (hex) of the archive at --url.")
	fs.StringVar(&o.dir, "dir", o.dir, "Directory to install the Terraform binary and ready marker into.")
	fs.StringVar(&o.legacyDir, "legacy-dir", "", "Optional directory that also receives a copy of the binary and a .terraform-source marker.")
	fs.DurationVar(&o.timeout, "timeout", o.timeout, "Maximum time to download and verify Terraform.")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		return o, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return o, nil
}

// validate checks the options and returns the requested release version, or nil when --url is used.
func (o options) validate() (*version.Version, error) {
	switch {
	case o.version == "" && o.url == "":
		return nil, errors.New("one of --version or --url is required")
	case o.version != "" && o.url != "":
		return nil, errors.New("--version and --url are mutually exclusive")
	case o.sha256 != "" && o.url == "":
		return nil, errors.New("--sha256 can only be used with --url")
	case o.dir == "":
		return nil, errors.New("--dir is required")
	case o.timeout <= 0:
		return nil, errors.New("--timeout must be greater than zero")
	}

	if o.url != "" {
		u, err := url.Parse(o.url)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			return nil, errors.New("--url must be an absolute http or https URL")
		}
		if o.sha256 != "" {
			if b, err := hex.DecodeString(o.sha256); err != nil || len(b) != 32 {
				return nil, errors.New("--sha256 must be a 64-character hex string")
			}
		}
		return nil, nil
	}

	v, err := version.NewVersion(o.version)
	if err != nil {
		return nil, fmt.Errorf("invalid Terraform version %q: %w", o.version, err)
	}
	return v, nil
}

// installTerraform installs Terraform into o.dir. When want is set, the binary must report that version.
func installTerraform(ctx context.Context, o options, want *version.Version, logger *slog.Logger, download downloader, verify verifier) error {
	if err := os.MkdirAll(o.dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", o.dir, err)
	}

	binary := filepath.Join(o.dir, tfbinary.BinaryName)
	start := time.Now()
	if want != nil {
		logger.Info("Downloading Terraform release", "terraformVersion", want.String(), "path", binary)
	} else {
		logger.Info("Downloading Terraform from custom URL", "url", o.url, "checksumVerified", o.sha256 != "", "path", binary)
	}
	if err := download(ctx, binary); err != nil {
		return err
	}
	logger.Info("Downloaded Terraform", "duration", time.Since(start).String())

	got, err := verify(ctx, binary)
	if err != nil {
		return fmt.Errorf("failed to verify Terraform binary: %w", err)
	}
	gotVersion, err := version.NewVersion(got)
	if err != nil {
		return fmt.Errorf("failed to verify Terraform binary: invalid version %q: %w", got, err)
	}
	if want != nil && !gotVersion.Equal(want) {
		return fmt.Errorf("failed to verify Terraform binary: expected version %s, got %s", want, gotVersion)
	}

	if o.legacyDir != "" {
		if err := tfbinary.CopyExecutable(binary, filepath.Join(o.legacyDir, tfbinary.BinaryName)); err != nil {
			return fmt.Errorf("failed to copy Terraform binary to %s: %w", o.legacyDir, err)
		}
		if err := os.WriteFile(filepath.Join(o.legacyDir, legacySourceFile), []byte(legacySource+"\n"), 0644); err != nil {
			return fmt.Errorf("failed to write legacy Terraform marker: %w", err)
		}
	}

	// The ready marker is written last so the resource providers only use a verified binary. It records
	// the installed version for troubleshooting.
	if err := os.WriteFile(filepath.Join(o.dir, tfbinary.MarkerFileName), []byte(gotVersion.String()+"\n"), 0644); err != nil {
		return fmt.Errorf("failed to write Terraform ready marker: %w", err)
	}

	logger.Info("Terraform is ready", "terraformVersion", gotVersion.String(), "path", binary)
	return nil
}

// terraformVersion runs "terraform version -json" and returns the reported version.
func terraformVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "version", "-json").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("%w: %s", err, exitErr.Stderr)
		}
		return "", err
	}

	var result struct {
		TerraformVersion string `json:"terraform_version"`
	}
	if err := json.Unmarshal(out, &result); err != nil || result.TerraformVersion == "" {
		return "", fmt.Errorf("unexpected output from terraform version -json: %s", out)
	}
	return result.TerraformVersion, nil
}
