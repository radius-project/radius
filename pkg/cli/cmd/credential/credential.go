// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"github.com/project-radius/radius/pkg/cli/cmd/credential/common"
	credential_list "github.com/project-radius/radius/pkg/cli/cmd/credential/list"
	credential_register "github.com/project-radius/radius/pkg/cli/cmd/credential/register"
	credential_show "github.com/project-radius/radius/pkg/cli/cmd/credential/show"
	credential_unregister "github.com/project-radius/radius/pkg/cli/cmd/credential/unregister"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command for the `rad credential` command.
func NewCommand(factory framework.Factory) *cobra.Command {
	// This command is not runnable, and thus has no runner.
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Manage cloud provider credential for a Radius installation.",
		Long:  "Manage cloud provider credential for a Radius installation." + common.LongDescriptionBlurb,
		Example: `
# List configured cloud providers credential
rad credential list

# Register (Add or Update) cloud provider credential for Azure with service principal authentication
rad credential register azure --client-id <client id> --client-secret <client secret> --tenant-id <tenant id> --subscription <subscription id> --resource-group <resource group name>

# Show cloud provider credential details for Azure
rad credential show azure

# Delete Azure cloud provider configuration
rad credential unregister azure
`,
	}

	create := credential_register.NewCommand(factory)
	cmd.AddCommand(create)

	delete, _ := credential_unregister.NewCommand(factory)
	cmd.AddCommand(delete)

	list, _ := credential_list.NewCommand(factory)
	cmd.AddCommand(list)

	show, _ := credential_show.NewCommand(factory)
	cmd.AddCommand(show)

	return cmd
}
