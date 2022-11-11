// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"github.com/project-radius/radius/pkg/cli/cmd/provider/common"
	provider_create "github.com/project-radius/radius/pkg/cli/cmd/provider/create"
	provider_delete "github.com/project-radius/radius/pkg/cli/cmd/provider/delete"
	provider_list "github.com/project-radius/radius/pkg/cli/cmd/provider/list"
	provider_show "github.com/project-radius/radius/pkg/cli/cmd/provider/show"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command for the `rad provider` command.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		// These commands are being built in a few steps. We'll show them when they are ready.
		Hidden: true,

		Use:   "provider",
		Short: "Manage cloud provider configuration for a Radius installation.",
		Long:  "Manage cloud provider configuration for a Radius installation." + common.LongDescriptionBlurb,
		Example: `
# List configured cloud providers
rad provider list

# Add cloud provider configuration for Azure with service principal authentication
rad provider create azure --client-id <client id> --client-secret <client secret> --tenant-id <tenant id> --subscription <subscription id> --resource-group <resource group name>

# Show cloud providers details for Azure
rad provider show azure

# Delete Azure cloud provider configuration
rad provider delete azure
`,
	}

	create := provider_create.NewCommand(factory)
	cmd.AddCommand(create)

	delete, _ := provider_delete.NewCommand(factory)
	cmd.AddCommand(delete)

	list, _ := provider_list.NewCommand(factory)
	cmd.AddCommand(list)

	show, _ := provider_show.NewCommand(factory)
	cmd.AddCommand(show)

	return cmd
}
