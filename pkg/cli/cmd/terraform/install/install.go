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

package install

import (
	"context"
	"os"
	"time"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/terraform/common"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/terraform/installer"
	"github.com/spf13/cobra"
)

const (
	// DefaultTimeout is the default timeout for waiting for installation to complete.
	DefaultTimeout = 10 * time.Minute

	// DefaultPollInterval is the default interval for polling installation status.
	DefaultPollInterval = 2 * time.Second
)

// NewCommand creates an instance of the `rad terraform install` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install Terraform for use with Radius recipes",
		Long:  "Install Terraform for use with Radius recipes. Terraform is downloaded and managed by Radius.",
		Example: `
# Install a specific version of Terraform
rad terraform install --version 1.6.4

# Install Terraform and wait for completion
rad terraform install --version 1.6.4 --wait

# Install Terraform from a custom URL
rad terraform install --url https://example.com/terraform.zip

# Install Terraform from a custom URL with checksum verification
rad terraform install --url https://example.com/terraform.zip --checksum sha256:abc123...

# Install from a private registry with a custom CA bundle
rad terraform install --url https://internal.example.com/terraform.zip --ca-bundle /path/to/ca.pem

# Install from a private registry with authentication
rad terraform install --url https://internal.example.com/terraform.zip --auth-header "Bearer <token>"

# Install from a private registry with mTLS client certificate
rad terraform install --url https://internal.example.com/terraform.zip --client-cert /path/to/cert.pem --client-key /path/to/key.pem

# Install through a corporate proxy
rad terraform install --url https://releases.hashicorp.com/terraform/1.6.4/terraform_1.6.4_linux_amd64.zip --proxy http://proxy.corp.com:8080

# Install with a custom timeout (when using --wait)
rad terraform install --version 1.6.4 --wait --timeout 15m
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	cmd.Flags().String("version", "", "The Terraform version to install (e.g., 1.6.4)")
	cmd.Flags().String("url", "", "The URL to download Terraform from (alternative to --version)")
	cmd.Flags().String("checksum", "", "The checksum to verify the download (format: sha256:<hash>)")
	cmd.Flags().Bool("wait", false, "Wait for the installation to complete")
	cmd.Flags().Duration("timeout", DefaultTimeout, "Timeout when waiting for installation (requires --wait)")
	cmd.Flags().String("ca-bundle", "", "Path to a PEM-encoded CA bundle file for TLS verification with private registries")
	cmd.Flags().String("auth-header", "", "HTTP Authorization header value (e.g., \"Bearer <token>\" or \"Basic <base64>\")")
	cmd.Flags().String("client-cert", "", "Path to a PEM-encoded client certificate for mTLS authentication")
	cmd.Flags().String("client-key", "", "Path to a PEM-encoded client private key for mTLS authentication")
	cmd.Flags().String("proxy", "", "HTTP/HTTPS proxy URL (e.g., \"http://proxy.corp.com:8080\")")

	return cmd, runner
}

// Runner is the runner implementation for the `rad terraform install` command.
type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
	Workspace    *workspaces.Workspace

	Version      string
	SourceURL    string
	Checksum     string
	Wait         bool
	Timeout      time.Duration
	CABundle     string
	AuthHeader   string
	ClientCert   string
	ClientKey    string
	ProxyURL     string
	PollInterval time.Duration
}

// NewRunner creates a new instance of the `rad terraform install` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		PollInterval: DefaultPollInterval,
	}
}

// Validate runs validation for the `rad terraform install` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.Version, err = cmd.Flags().GetString("version")
	if err != nil {
		return err
	}

	r.SourceURL, err = cmd.Flags().GetString("url")
	if err != nil {
		return err
	}

	r.Checksum, err = cmd.Flags().GetString("checksum")
	if err != nil {
		return err
	}

	r.Wait, err = cmd.Flags().GetBool("wait")
	if err != nil {
		return err
	}

	r.Timeout, err = cmd.Flags().GetDuration("timeout")
	if err != nil {
		return err
	}

	r.CABundle, err = cmd.Flags().GetString("ca-bundle")
	if err != nil {
		return err
	}

	r.AuthHeader, err = cmd.Flags().GetString("auth-header")
	if err != nil {
		return err
	}

	r.ClientCert, err = cmd.Flags().GetString("client-cert")
	if err != nil {
		return err
	}

	r.ClientKey, err = cmd.Flags().GetString("client-key")
	if err != nil {
		return err
	}

	r.ProxyURL, err = cmd.Flags().GetString("proxy")
	if err != nil {
		return err
	}

	// Validate that at least one of --version or --url is provided
	if r.Version == "" && r.SourceURL == "" {
		return clierrors.Message("Either --version or --url must be specified.")
	}

	// Validate that --version is required when using --wait (server generates a version hash from URL which cannot be predicted)
	if r.Wait && r.Version == "" {
		return clierrors.Message("--version is required when using --wait (the server generates a version hash from the URL which cannot be predicted).")
	}

	// Validate that --timeout requires --wait
	if cmd.Flags().Changed("timeout") && !r.Wait {
		return clierrors.Message("--timeout requires --wait to be set.")
	}

	// Validate that --ca-bundle requires --url (only makes sense for custom URLs)
	if r.CABundle != "" && r.SourceURL == "" {
		return clierrors.Message("--ca-bundle requires --url to be set.")
	}

	// Validate that --auth-header requires --url
	if r.AuthHeader != "" && r.SourceURL == "" {
		return clierrors.Message("--auth-header requires --url to be set.")
	}

	// Validate that --client-cert and --client-key must be used together
	if (r.ClientCert != "" && r.ClientKey == "") || (r.ClientCert == "" && r.ClientKey != "") {
		return clierrors.Message("--client-cert and --client-key must be specified together.")
	}

	// Validate that --client-cert requires --url
	if r.ClientCert != "" && r.SourceURL == "" {
		return clierrors.Message("--client-cert requires --url to be set.")
	}

	// Validate that --proxy requires --url
	if r.ProxyURL != "" && r.SourceURL == "" {
		return clierrors.Message("--proxy requires --url to be set.")
	}

	return nil
}

// Run runs the `rad terraform install` command.
func (r *Runner) Run(ctx context.Context) error {
	connection, err := r.Workspace.Connect(ctx)
	if err != nil {
		return err
	}

	client := common.NewClient(connection)

	req := installer.InstallRequest{
		Version:    r.Version,
		SourceURL:  r.SourceURL,
		Checksum:   r.Checksum,
		AuthHeader: r.AuthHeader,
		ProxyURL:   r.ProxyURL,
	}

	// Read CA bundle file if specified
	if r.CABundle != "" {
		caBytes, err := os.ReadFile(r.CABundle)
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to read CA bundle file %q.", r.CABundle)
		}
		req.CABundle = string(caBytes)
	}

	// Read client certificate file if specified
	if r.ClientCert != "" {
		certBytes, err := os.ReadFile(r.ClientCert)
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to read client certificate file %q.", r.ClientCert)
		}
		req.ClientCert = string(certBytes)
	}

	// Read client key file if specified
	if r.ClientKey != "" {
		keyBytes, err := os.ReadFile(r.ClientKey)
		if err != nil {
			return clierrors.MessageWithCause(err, "Failed to read client key file %q.", r.ClientKey)
		}
		req.ClientKey = string(keyBytes)
	}

	r.Output.LogInfo("Installing Terraform...")

	if err := client.Install(ctx, req); err != nil {
		return err
	}

	versionInfo := r.Version
	if versionInfo == "" {
		versionInfo = r.SourceURL
	}
	r.Output.LogInfo("Terraform install queued (version=%s)", versionInfo)

	if r.Wait {
		return r.waitForInstallation(ctx, client)
	}

	return nil
}

// waitForInstallation polls the status endpoint until the installation completes or fails.
func (r *Runner) waitForInstallation(ctx context.Context, client *common.Client) error {
	r.Output.LogInfo("Waiting for installation to complete...")

	deadline := time.Now().Add(r.Timeout)
	pollInterval := r.PollInterval

	for {
		if time.Now().After(deadline) {
			return clierrors.Message("Timed out waiting for Terraform installation to complete.")
		}

		status, err := client.Status(ctx)
		if err != nil {
			return err
		}

		// Check if the target version is installed
		if vs, ok := status.Versions[r.Version]; ok {
			switch vs.State {
			case installer.VersionStateSucceeded:
				if status.CurrentVersion == r.Version {
					r.Output.LogInfo("Terraform %s installed successfully.", r.Version)
					return nil
				}
				// Version succeeded but isn't current - this is an unexpected state.
				// The server always sets current version when marking succeeded, so this
				// indicates a bug or race condition. Return an error rather than polling forever.
				return clierrors.Message("Terraform %s installed but not set as current version (current: %s). This may indicate a server-side issue.", r.Version, status.CurrentVersion)
			case installer.VersionStateFailed:
				if vs.LastError != "" {
					return clierrors.Message("Terraform installation failed: %s", vs.LastError)
				}
				return clierrors.Message("Terraform installation failed.")
			}
		}

		// Check overall state for failures (e.g., server fails before populating version status)
		if status.State == installer.ResponseStateFailed {
			if status.LastError != "" {
				return clierrors.Message("Terraform installation failed: %s", status.LastError)
			}
			return clierrors.Message("Terraform installation failed.")
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling
		}
	}
}
