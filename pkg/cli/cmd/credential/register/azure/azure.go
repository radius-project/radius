// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	"github.com/project-radius/radius/pkg/cli/connections"
	cli_credential "github.com/project-radius/radius/pkg/cli/credential"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	ucp "github.com/project-radius/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad provider create azure` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "azure",
		Short: "Register (Add or update) Azure cloud provider credential for a Radius installation.",
		Long: `Register (Add or update) Azure cloud provider credential for a Radius installation..

This command is intended for scripting or advanced use-cases. See 'rad init' for a user-friendly way
to configure these settings.

Radius will use the provided service principal for all interations with Azure, including Bicep deployment, 
Radius environments, and Radius links. 

Radius will use the provided subscription and resource group as the default target scope for Bicep deployment.
The provided service principal must have the Contributor or Owner role assigned for the provided resource group
in order to create or manage resources contained in the group. The resource group should be created before
calling 'rad provider create azure'.
` + common.LongDescriptionBlurb,
		Example: `
# Register (Add or update) cloud provider credential for Azure with service principal authentication
rad credential register azure --client-id <client id/app id> --client-secret <client secret/password> --tenant-id <tenant id> --subscription <subscription id> --resource-group <resource group name>		
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	cmd.Flags().String("client-id", "", "The client id or app id of an Azure service principal.")
	_ = cmd.MarkFlagRequired("client-id")

	cmd.Flags().String("client-secret", "", "The client secret or password of an Azure service principal.")
	_ = cmd.MarkFlagRequired("client-secret")

	cmd.Flags().String("tenant-id", "", "The tenant id of an Azure service principal.")
	_ = cmd.MarkFlagRequired("tenant-id")

	cmd.Flags().String("subscription", "", "The subscription id of the target Azure subscription. The subscription id will be stored in local configuration and used by 'rad deploy'.")
	_ = cmd.MarkFlagRequired("subscription")

	cmd.Flags().String("resource-group", "", "The resource group name of an existing Azure resource group. The resource group will be stored in local configuration and used by 'rad deploy'.")
	_ = cmd.MarkFlagRequired("resource-group")

	return cmd, runner
}

// Runner is the runner implementation for the `rad provider create azure` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	ClientID     string
	ClientSecret string
	TenantID     string
	//TODO: move scope components out to provider commands
	SubscriptionID string
	ResourceGroup  string
	KubeContext    string
}

// NewRunner creates a new instance of the `rad credential register azure` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad credential register azure` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	// TODO: support fallback workspace
	if !r.Workspace.IsNamedWorkspace() {
		return workspaces.ErrNamedWorkspaceRequired
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	clientID, err := cmd.Flags().GetString("client-id")
	if err != nil {
		return err
	}
	clientSecret, err := cmd.Flags().GetString("client-secret")
	if err != nil {
		return err
	}
	tenantID, err := cmd.Flags().GetString("tenant-id")
	if err != nil {
		return err
	}
	subscriptionID, err := cmd.Flags().GetString("subscription")
	if err != nil {
		return err
	}
	resourceGroup, err := cmd.Flags().GetString("resource-group")
	if err != nil {
		return err
	}

	r.ClientID = clientID
	r.ClientSecret = clientSecret
	r.TenantID = tenantID
	r.SubscriptionID = subscriptionID
	r.ResourceGroup = resourceGroup

	valid, message, _ := prompt.UUIDv4Validator(r.SubscriptionID)
	if !valid {
		return &cli.FriendlyError{Message: fmt.Sprintf("Subscription id %q is invalid: %s", r.SubscriptionID, message)}
	}

	kubeContext, ok := r.Workspace.KubernetesContext()
	if !ok {
		return &cli.FriendlyError{Message: "A Kubernetes connection is required."}
	}
	r.KubeContext = kubeContext

	return nil
}

// Run runs the `rad credential register azure` command.
func (r *Runner) Run(ctx context.Context) error {
	// There are two steps to perform here:
	// 1) Update server-side to add/change credentials
	// 2) Update local config (all matching workspaces) to remove the scope

	r.Output.LogInfo("Configuring credential for cloud provider %q for Radius installation %q...", "azure", r.Workspace.FmtConnection())
	client, err := r.ConnectionFactory.CreateCredentialManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	credential := ucp.AzureCredentialResource{
		Location: to.Ptr(v1.LocationGlobal),
		Type:     to.Ptr(cli_credential.AzureCredential),
		ID:       to.Ptr(fmt.Sprintf(common.AzureCredentialID, "default")),
		Properties: &ucp.AzureServicePrincipalProperties{
			Storage: &ucp.CredentialStorageProperties{
				Kind: to.Ptr(string(ucp.CredentialStorageKindInternal)),
			},
			TenantID:     &r.TenantID,
			ClientID:     &r.ClientID,
			ClientSecret: &r.ClientSecret,
			Kind:         to.Ptr("ServicePrincipal"),
		},
	}

	// 1) Update server-side to add/change credentials
	err = client.PutAzure(ctx, credential)
	if err != nil {
		return err
	}

	// 2) Update local config (all matching workspaces) to remove the scope
	// TODO: move updating scope to provider commands
	err = cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
		cli.UpdateAzProvider(section, workspaces.AzureProvider{SubscriptionID: r.SubscriptionID, ResourceGroup: r.ResourceGroup}, r.KubeContext)
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
