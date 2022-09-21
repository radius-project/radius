// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package group

import (
	group_create "github.com/project-radius/radius/pkg/cli/cmd/group/create"
	group_delete "github.com/project-radius/radius/pkg/cli/cmd/group/delete"
	group_switch "github.com/project-radius/radius/pkg/cli/cmd/group/groupswitch"
	group_list "github.com/project-radius/radius/pkg/cli/cmd/group/list"
	group_show "github.com/project-radius/radius/pkg/cli/cmd/group/show"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		// These commands are being built in a few steps. We'll show them when they are ready.
		Hidden: true,

		Use:   "group",
		Short: "Manage resource groups",
		Long:  "`Manage resource groups. This is NOT the same as Azure resource groups.`",
		Example: `
# List resource groups in workspace
rad group list

# create resource group in workspace
rad group create azure --client-id <client id> --client-secret <client secret> --tenant-id <tenant id> --subscription <subscription id> --resource-group <resource group name>

# Show cloud providers details for Azure
rad provider show azure

# Delete Azure cloud provider configuration
rad provider delete azure
`,
	}

	create, _ := group_create.NewCommand(factory)
	cmd.AddCommand(create)

	delete, _ := group_delete.NewCommand(factory)
	cmd.AddCommand(delete)

	list, _ := group_list.NewCommand(factory)
	cmd.AddCommand(list)

	show, _ := group_show.NewCommand(factory)
	cmd.AddCommand(show)

	groupswitch, _ := group_switch.NewCommand(factory)
	cmd.AddCommand(groupswitch)

	return cmd

}
