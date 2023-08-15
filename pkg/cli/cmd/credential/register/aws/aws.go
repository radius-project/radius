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

package aws

import (
	"context"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clierrors"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/to"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad credential register aws` command.
//
// # Function Explanation
//
// NewCommand creates a new cobra command for registering AWS cloud provider credentials with IAM authentication, and
// returns a Runner to execute the command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "aws",
		Short: "Register (Add or update) AWS cloud provider credential for a Radius installation.",
		Long: `Register (Add or update) AWS cloud provider credential for a Radius installation..

This command is intended for scripting or advanced use-cases. See 'rad init' for a user-friendly way
to configure these settings.

Radius will use the provided IAM credential for all interactions with AWS. 
` + common.LongDescriptionBlurb,
		Example: `
# Register (Add or update) cloud provider credential for AWS with IAM authentication
rad credential register aws --access-key-id <access-key-id> --secret-access-key <secret-access-key>
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	cmd.Flags().String("access-key-id", "", "The AWS IAM access key id.")
	_ = cmd.MarkFlagRequired("access-key-id")

	cmd.Flags().String("secret-access-key", "", "The AWS IAM secret access key.")
	_ = cmd.MarkFlagRequired("secret-access-key")

	return cmd, runner
}

// Runner is the runner implementation for the `rad credential register aws` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	AccessKeyID     string
	SecretAccessKey string
	KubeContext     string
}

// NewRunner creates a new instance of the `rad credential register aws` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad credential register aws` command.
//
// # Function Explanation
//
// Validate() checks if the required workspace, output format, access key ID and secret access key are present, and if not, returns an error.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	accessKeyID, err := cmd.Flags().GetString("access-key-id")
	if err != nil {
		return err
	}
	secretAccessKey, err := cmd.Flags().GetString("secret-access-key")
	if err != nil {
		return err
	}
	r.AccessKeyID = accessKeyID
	r.SecretAccessKey = secretAccessKey

	if r.AccessKeyID == "" {
		return clierrors.Message("Access Key id %q cannot be empty.", r.AccessKeyID)
	}
	if r.SecretAccessKey == "" {
		return clierrors.Message("Secret Access Key %q cannot be empty.", r.SecretAccessKey)
	}

	kubeContext, ok := r.Workspace.KubernetesContext()
	if !ok {
		return clierrors.Message("A Kubernetes connection is required.")
	}
	r.KubeContext = kubeContext
	return nil
}

// Run runs the `rad credential register aws` command.
//
// # Function Explanation
//
// Run() registers an AWS credential with the given context and workspace, and returns an error if unsuccessful.
func (r *Runner) Run(ctx context.Context) error {

	r.Output.LogInfo("Registering credential for %q cloud provider in Radius installation %q...", "aws", r.Workspace.FmtConnection())
	client, err := r.ConnectionFactory.CreateCredentialManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}
	credential := ucp.AWSCredentialResource{
		Location: to.Ptr(v1.LocationGlobal),
		Type:     to.Ptr(cli_credential.AWSCredential),
		Properties: &ucp.AWSAccessKeyCredentialProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(string(ucp.CredentialStorageKindInternal)),
			},
			AccessKeyID:     &r.AccessKeyID,
			SecretAccessKey: &r.SecretAccessKey,
		},
	}

	err = client.PutAWS(ctx, credential)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Successfully registered credential for %q cloud provider. Tokens may take up to 30 seconds to refresh.", "aws")

	return nil
}
