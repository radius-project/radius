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

package wi

import (
	"context"
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/credential/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	cli_credential "github.com/radius-project/radius/pkg/cli/credential"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/to"
	ucp "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"

	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad credential create azure wi` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "wi",
		Short: "Register (Add or update) Azure cloud provider workload identity credential for a Radius installation.",
		Long: `Register (Add or update) Azure cloud provider workload identity credential for a Radius installation.

This command is intended for scripting or advanced use-cases. See 'rad init' for a user-friendly way
to configure these settings.

Radius will use the provided workload identity credential for all interactions with Azure, including Bicep deployment, 
Radius Environments, and Radius portable resources.

Radius will use the provided subscription and resource group as the default target scope for Bicep deployment.
The provided service principal must have the Contributor or Owner role assigned for the provided resource group
in order to create or manage resources contained in the group. The resource group should be created before
calling 'rad credential register azure wi'.
` + common.LongDescriptionBlurb,
		Example: `
# Register (Add or update) cloud provider credential for Azure with workload identity authentication
rad credential register azure wi --client-id <client id/app id> --tenant-id <tenant id>
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	cmd.Flags().StringVar(&runner.ClientID, "client-id", "", "The client id or app id of an Azure service principal.")
	_ = cmd.MarkFlagRequired("client-id")

	cmd.Flags().StringVar(&runner.TenantID, "tenant-id", "", "The tenant id of an Azure service principal.")
	_ = cmd.MarkFlagRequired("tenant-id")

	return cmd, runner
}

// Runner is the runner implementation for the `rad credential register azure wi` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	ClientID    string
	TenantID    string
	KubeContext string
}

// NewRunner creates a new instance of the `rad credential register azure wi` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad credential register azure wi` command.
//

// Validate checks for the presence of a workspace, output format, client ID, and tenant ID, and
// sets them in the Runner struct if they are present. If any of these are not present, an error is returned.
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

	return nil
}

// Run runs the `rad credential register azure wi` command.
//

// Run registers a credential for the Azure cloud provider in the Radius installation, updates the server-side
// to add/change credentials. It returns an error if any of the steps fail.
func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Registering credential for %q cloud provider in Radius installation %q...", "azure", r.Workspace.FmtConnection())
	client, err := r.ConnectionFactory.CreateCredentialManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	credential := ucp.AzureCredentialResource{
		Location: to.Ptr(v1.LocationGlobal),
		Type:     to.Ptr(cli_credential.AzureCredential),
		ID:       to.Ptr(fmt.Sprintf(common.AzureCredentialID, "default")),
		Properties: &ucp.AzureWorkloadIdentityProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(ucp.CredentialStorageKindInternal),
			},
			TenantID: &r.TenantID,
			ClientID: &r.ClientID,
			Kind:     to.Ptr(ucp.AzureCredentialKindWorkloadIdentity),
		},
	}

	// Update server-side to add/change credentials
	err = client.PutAzure(ctx, credential)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Successfully registered credential for %q cloud provider. Tokens may take up to 30 seconds to refresh.", "azure")

	return nil
}
